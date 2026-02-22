package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	todoistKey    string
	todoistClient = &http.Client{Timeout: 15 * time.Second}
	todoistBase   = "https://api.todoist.com/rest/v2"
)

func initTodoist(key string) {
	todoistKey = key
}

func todoistRequest(method, path string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, todoistBase+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+todoistKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return todoistClient.Do(req)
}

func todoistReadJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Todoist API error (%d): %s", resp.StatusCode, string(body))
	}
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, v)
}

func registerRoutes(r chi.Router) {
	r.Get("/tasks", handleListTasks)
	r.Post("/tasks", handleCreateTask)
	r.Get("/projects", handleListProjects)
	r.Post("/tasks/close", handleCloseTask)
}

func handleListTasks(w http.ResponseWriter, r *http.Request) {
	path := "/tasks"
	if projectID := r.URL.Query().Get("project_id"); projectID != "" {
		path += "?project_id=" + projectID
	}

	resp, err := todoistRequest("GET", path, nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Todoist API request failed")
		return
	}

	var tasks []map[string]interface{}
	if err := todoistReadJSON(resp, &tasks); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	results := make([]map[string]interface{}, 0, len(tasks))
	for _, task := range tasks {
		results = append(results, map[string]interface{}{
			"id":           task["id"],
			"content":      task["content"],
			"description":  task["description"],
			"project_id":   task["project_id"],
			"is_completed": task["is_completed"],
			"priority":     task["priority"],
			"due":          task["due"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count": len(results),
		"tasks": results,
	})
}

type createTaskRequest struct {
	Content   string `json:"content"`
	ProjectID string `json:"project_id,omitempty"`
	Priority  int    `json:"priority,omitempty"`
	DueString string `json:"due_string,omitempty"`
}

func handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req createTaskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	body := map[string]interface{}{
		"content": req.Content,
	}
	if req.ProjectID != "" {
		body["project_id"] = req.ProjectID
	}
	if req.Priority > 0 {
		body["priority"] = req.Priority
	}
	if req.DueString != "" {
		body["due_string"] = req.DueString
	}

	resp, err := todoistRequest("POST", "/tasks", body)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Todoist API request failed")
		return
	}

	var task map[string]interface{}
	if err := todoistReadJSON(resp, &task); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":         task["id"],
		"content":    task["content"],
		"project_id": task["project_id"],
	})
}

func handleListProjects(w http.ResponseWriter, r *http.Request) {
	resp, err := todoistRequest("GET", "/projects", nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Todoist API request failed")
		return
	}

	var projects []map[string]interface{}
	if err := todoistReadJSON(resp, &projects); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	results := make([]map[string]interface{}, 0, len(projects))
	for _, proj := range projects {
		results = append(results, map[string]interface{}{
			"id":          proj["id"],
			"name":        proj["name"],
			"color":       proj["color"],
			"is_favorite": proj["is_favorite"],
			"url":         proj["url"],
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count":    len(results),
		"projects": results,
	})
}

type closeTaskRequest struct {
	TaskID string `json:"task_id"`
}

func handleCloseTask(w http.ResponseWriter, r *http.Request) {
	var req closeTaskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TaskID == "" {
		writeError(w, http.StatusBadRequest, "task_id is required")
		return
	}

	resp, err := todoistRequest("POST", "/tasks/"+req.TaskID+"/close", nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Todoist API request failed")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Todoist API error (%d): %s", resp.StatusCode, string(body)))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"task_id": req.TaskID,
	})
}
