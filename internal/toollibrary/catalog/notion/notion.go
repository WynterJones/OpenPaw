package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	notionKey    string
	notionClient = &http.Client{Timeout: 15 * time.Second}
	notionBase   = "https://api.notion.com/v1"
)

func initNotion(key string) {
	notionKey = key
}

func notionRequest(method, path string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, notionBase+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+notionKey)
	req.Header.Set("Notion-Version", "2022-06-28")
	req.Header.Set("Content-Type", "application/json")
	return notionClient.Do(req)
}

func notionReadJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Notion API error (%d): %s", resp.StatusCode, string(data))
	}
	return json.Unmarshal(data, v)
}

// extractTitle extracts the plain-text title from a Notion properties object.
// Notion stores titles in a nested structure: properties.Name.title[0].plain_text
func extractTitle(props interface{}) string {
	properties, ok := props.(map[string]interface{})
	if !ok {
		return ""
	}

	// Try common title field names
	for _, key := range []string{"Name", "Title", "name", "title"} {
		prop, ok := properties[key].(map[string]interface{})
		if !ok {
			continue
		}
		titleArr, ok := prop["title"].([]interface{})
		if !ok || len(titleArr) == 0 {
			continue
		}
		first, ok := titleArr[0].(map[string]interface{})
		if !ok {
			continue
		}
		if text, ok := first["plain_text"].(string); ok {
			return text
		}
	}

	// Fallback: scan all properties for a title type
	for _, v := range properties {
		prop, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if prop["type"] == "title" {
			titleArr, ok := prop["title"].([]interface{})
			if !ok || len(titleArr) == 0 {
				continue
			}
			first, ok := titleArr[0].(map[string]interface{})
			if !ok {
				continue
			}
			if text, ok := first["plain_text"].(string); ok {
				return text
			}
		}
	}

	return ""
}

// simplifyProperties converts Notion's complex property structure into simple key-value pairs.
func simplifyProperties(props map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for key, v := range props {
		prop, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		propType, _ := prop["type"].(string)
		switch propType {
		case "title":
			if arr, ok := prop["title"].([]interface{}); ok && len(arr) > 0 {
				if first, ok := arr[0].(map[string]interface{}); ok {
					result[key] = first["plain_text"]
				}
			}
		case "rich_text":
			if arr, ok := prop["rich_text"].([]interface{}); ok && len(arr) > 0 {
				if first, ok := arr[0].(map[string]interface{}); ok {
					result[key] = first["plain_text"]
				}
			}
		case "number":
			result[key] = prop["number"]
		case "select":
			if sel, ok := prop["select"].(map[string]interface{}); ok {
				result[key] = sel["name"]
			}
		case "multi_select":
			if arr, ok := prop["multi_select"].([]interface{}); ok {
				names := make([]string, 0, len(arr))
				for _, item := range arr {
					if m, ok := item.(map[string]interface{}); ok {
						if name, ok := m["name"].(string); ok {
							names = append(names, name)
						}
					}
				}
				result[key] = names
			}
		case "date":
			if d, ok := prop["date"].(map[string]interface{}); ok {
				result[key] = d["start"]
			}
		case "checkbox":
			result[key] = prop["checkbox"]
		case "url":
			result[key] = prop["url"]
		case "email":
			result[key] = prop["email"]
		case "phone_number":
			result[key] = prop["phone_number"]
		case "status":
			if s, ok := prop["status"].(map[string]interface{}); ok {
				result[key] = s["name"]
			}
		default:
			result[key] = prop[propType]
		}
	}
	return result
}

func registerRoutes(r chi.Router) {
	r.Post("/search", handleSearch)
	r.Get("/page/{id}", handleGetPage)
	r.Post("/page", handleCreatePage)
	r.Get("/database/{id}", handleQueryDatabase)
	r.Get("/databases", handleListDatabases)
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query    string `json:"query"`
		PageSize int    `json:"page_size"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}

	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	resp, err := notionRequest("POST", "/search", map[string]interface{}{
		"query":     req.Query,
		"page_size": req.PageSize,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "Notion API request failed")
		return
	}

	var data map[string]interface{}
	if err := notionReadJSON(resp, &data); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	results, _ := data["results"].([]interface{})
	output := make([]map[string]interface{}, 0, len(results))
	for _, item := range results {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		title := ""
		if props, ok := obj["properties"].(map[string]interface{}); ok {
			title = extractTitle(props)
		}

		output = append(output, map[string]interface{}{
			"id":               obj["id"],
			"object_type":      obj["object"],
			"title":            title,
			"url":              obj["url"],
			"created_time":     obj["created_time"],
			"last_edited_time": obj["last_edited_time"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":   len(output),
		"results": output,
	})
}

func handleGetPage(w http.ResponseWriter, r *http.Request) {
	pageID := chi.URLParam(r, "id")
	if pageID == "" {
		writeError(w, http.StatusBadRequest, "page id is required")
		return
	}

	resp, err := notionRequest("GET", "/pages/"+pageID, nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Notion API request failed")
		return
	}

	var page map[string]interface{}
	if err := notionReadJSON(resp, &page); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	properties := make(map[string]interface{})
	if props, ok := page["properties"].(map[string]interface{}); ok {
		properties = simplifyProperties(props)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":               page["id"],
		"url":              page["url"],
		"created_time":     page["created_time"],
		"last_edited_time": page["last_edited_time"],
		"properties":       properties,
	})
}

type createPageRequest struct {
	ParentID   string `json:"parent_id"`
	ParentType string `json:"parent_type"`
	Title      string `json:"title"`
	Content    string `json:"content"`
}

func handleCreatePage(w http.ResponseWriter, r *http.Request) {
	var req createPageRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ParentID == "" || req.ParentType == "" || req.Title == "" {
		writeError(w, http.StatusBadRequest, "parent_id, parent_type, and title are required")
		return
	}

	if req.ParentType != "database_id" && req.ParentType != "page_id" {
		writeError(w, http.StatusBadRequest, "parent_type must be 'database_id' or 'page_id'")
		return
	}

	payload := map[string]interface{}{
		"parent": map[string]interface{}{
			req.ParentType: req.ParentID,
		},
	}

	// For database parents, set title via properties
	if req.ParentType == "database_id" {
		payload["properties"] = map[string]interface{}{
			"Name": map[string]interface{}{
				"title": []map[string]interface{}{
					{
						"text": map[string]interface{}{
							"content": req.Title,
						},
					},
				},
			},
		}
	} else {
		// For page parents, set title via properties with title type
		payload["properties"] = map[string]interface{}{
			"title": map[string]interface{}{
				"title": []map[string]interface{}{
					{
						"text": map[string]interface{}{
							"content": req.Title,
						},
					},
				},
			},
		}
	}

	// Add content as paragraph blocks if provided
	if req.Content != "" {
		payload["children"] = []map[string]interface{}{
			{
				"object": "block",
				"type":   "paragraph",
				"paragraph": map[string]interface{}{
					"rich_text": []map[string]interface{}{
						{
							"type": "text",
							"text": map[string]interface{}{
								"content": req.Content,
							},
						},
					},
				},
			},
		}
	}

	resp, err := notionRequest("POST", "/pages", payload)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Notion API request failed")
		return
	}

	var page map[string]interface{}
	if err := notionReadJSON(resp, &page); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":  page["id"],
		"url": page["url"],
	})
}

func handleQueryDatabase(w http.ResponseWriter, r *http.Request) {
	dbID := chi.URLParam(r, "id")
	if dbID == "" {
		writeError(w, http.StatusBadRequest, "database id is required")
		return
	}

	pageSize := 25
	if ps := r.URL.Query().Get("page_size"); ps != "" {
		if n, err := strconv.Atoi(ps); err == nil && n >= 1 && n <= 100 {
			pageSize = n
		}
	}

	resp, err := notionRequest("POST", "/databases/"+dbID+"/query", map[string]interface{}{
		"page_size": pageSize,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "Notion API request failed")
		return
	}

	var data map[string]interface{}
	if err := notionReadJSON(resp, &data); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	results, _ := data["results"].([]interface{})
	output := make([]map[string]interface{}, 0, len(results))
	for _, item := range results {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		entry := map[string]interface{}{
			"id":  obj["id"],
			"url": obj["url"],
		}

		if props, ok := obj["properties"].(map[string]interface{}); ok {
			entry["properties"] = simplifyProperties(props)
		}

		output = append(output, entry)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"database_id": dbID,
		"count":       len(output),
		"results":     output,
	})
}

func handleListDatabases(w http.ResponseWriter, r *http.Request) {
	resp, err := notionRequest("POST", "/search", map[string]interface{}{
		"filter": map[string]interface{}{
			"value":    "database",
			"property": "object",
		},
		"page_size": 100,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "Notion API request failed")
		return
	}

	var data map[string]interface{}
	if err := notionReadJSON(resp, &data); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	results, _ := data["results"].([]interface{})
	output := make([]map[string]interface{}, 0, len(results))
	for _, item := range results {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		title := ""
		if titleArr, ok := obj["title"].([]interface{}); ok && len(titleArr) > 0 {
			if first, ok := titleArr[0].(map[string]interface{}); ok {
				title, _ = first["plain_text"].(string)
			}
		}

		output = append(output, map[string]interface{}{
			"id":    obj["id"],
			"title": title,
			"url":   obj["url"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":     len(output),
		"databases": output,
	})
}
