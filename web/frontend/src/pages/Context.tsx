import { useState, useEffect, useRef, useCallback } from "react";
import {
  Folder,
  FolderOpen,
  File,
  Upload,
  FolderPlus,
  Trash2,
  ChevronRight,
  ChevronDown,
  Save,
  FileImage,
  FileText,
  Database,
  UserPen,
  Pencil,
  GripVertical,
  PanelLeft,
} from "lucide-react";
import { Header } from "../components/Header";
import { Button } from "../components/Button";
import { Modal } from "../components/Modal";
import { useToast } from "../components/Toast";
import {
  contextApi,
  type ContextFile,
  type ContextTree,
  type ContextTreeNode,
} from "../lib/api";

// ---- Utilities ----------------------------------------------------------------

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function isTextMime(mime: string): boolean {
  return (
    mime.startsWith("text/") ||
    mime === "application/json" ||
    mime === "application/xml" ||
    mime === "application/javascript"
  );
}

function isImageMime(mime: string): boolean {
  return mime.startsWith("image/");
}

function MimeIcon({ mime, className }: { mime: string; className?: string }) {
  if (isImageMime(mime)) return <FileImage className={className} />;
  if (isTextMime(mime)) return <FileText className={className} />;
  return <Database className={className} />;
}

// ---- Context Menu Component ---------------------------------------------------

interface ContextMenuState {
  x: number;
  y: number;
  file: ContextFile;
}

function ContextMenu({
  menu,
  onRename,
  onDelete,
  onClose,
}: {
  menu: ContextMenuState;
  onRename: (file: ContextFile) => void;
  onDelete: (file: ContextFile) => void;
  onClose: () => void;
}) {
  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", handleKey);
    return () => window.removeEventListener("keydown", handleKey);
  }, [onClose]);

  return (
    <>
      <div className="fixed inset-0 z-50" onClick={onClose} />
      <div
        className="fixed z-50 min-w-[140px] rounded-lg border border-border-1 bg-surface-2 shadow-lg py-1"
        style={{ left: menu.x, top: menu.y }}
      >
        <button
          className="w-full flex items-center gap-2 px-3 py-1.5 text-sm text-text-1 hover:bg-surface-3 transition-colors cursor-pointer"
          onClick={() => {
            onRename(menu.file);
            onClose();
          }}
        >
          <Pencil className="w-3.5 h-3.5" />
          Rename
        </button>
        <button
          className="w-full flex items-center gap-2 px-3 py-1.5 text-sm text-red-400 hover:bg-surface-3 transition-colors cursor-pointer"
          onClick={() => {
            onDelete(menu.file);
            onClose();
          }}
        >
          <Trash2 className="w-3.5 h-3.5" />
          Delete
        </button>
      </div>
    </>
  );
}

// ---- Sub-components -----------------------------------------------------------

interface TreeNodeProps {
  node: ContextTreeNode;
  level: number;
  expanded: Set<string>;
  selectedFileId: string | null;
  onToggleFolder: (id: string) => void;
  onSelectFile: (file: ContextFile) => void;
  onDeleteFolder: (id: string, name: string) => void;
  onFileDrop: (fileId: string, folderId: string) => void;
  onContextMenu: (e: React.MouseEvent, file: ContextFile) => void;
}

function TreeNodeItem({
  node,
  level,
  expanded,
  selectedFileId,
  onToggleFolder,
  onSelectFile,
  onDeleteFolder,
  onFileDrop,
  onContextMenu,
}: TreeNodeProps) {
  const isOpen = expanded.has(node.id);
  const [dragOver, setDragOver] = useState(false);

  const handleDragOver = (e: React.DragEvent) => {
    if (e.dataTransfer.types.includes("application/x-context-file")) {
      e.preventDefault();
      e.dataTransfer.dropEffect = "move";
      setDragOver(true);
    }
  };

  const handleDragLeave = () => setDragOver(false);

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    const fileId = e.dataTransfer.getData("application/x-context-file");
    if (fileId) onFileDrop(fileId, node.id);
  };

  return (
    <div role="treeitem" aria-expanded={isOpen} aria-level={level + 1}>
      <div
        className={`relative group flex items-center gap-1.5 rounded-lg px-2 py-1.5 cursor-pointer hover:bg-surface-2 transition-colors text-sm text-text-1 ${
          dragOver ? "ring-2 ring-accent-primary bg-accent-muted/30" : ""
        }`}
        style={{ paddingLeft: `${8 + level * 16}px` }}
        onClick={() => onToggleFolder(node.id)}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
      >
        {isOpen ? (
          <ChevronDown className="w-3.5 h-3.5 text-text-3 flex-shrink-0" />
        ) : (
          <ChevronRight className="w-3.5 h-3.5 text-text-3 flex-shrink-0" />
        )}
        {isOpen ? (
          <FolderOpen className="w-4 h-4 text-accent-primary flex-shrink-0" />
        ) : (
          <Folder className="w-4 h-4 text-text-2 flex-shrink-0" />
        )}
        <span className="flex-1 truncate">{node.name}</span>
        <button
          onClick={(e) => {
            e.stopPropagation();
            onDeleteFolder(node.id, node.name);
          }}
          className="opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 focus:opacity-100 p-0.5 rounded text-text-3 hover:text-red-400 transition-all cursor-pointer flex-shrink-0"
          title="Delete folder"
          aria-label="Delete folder"
        >
          <Trash2 className="w-3.5 h-3.5" />
        </button>
      </div>

      {isOpen && (
        <div role="group">
          {node.children.map((child) => (
            <TreeNodeItem
              key={child.id}
              node={child}
              level={level + 1}
              expanded={expanded}
              selectedFileId={selectedFileId}
              onToggleFolder={onToggleFolder}
              onSelectFile={onSelectFile}
              onDeleteFolder={onDeleteFolder}
              onFileDrop={onFileDrop}
              onContextMenu={onContextMenu}
            />
          ))}
          {node.files.map((file) => (
            <FileItem
              key={file.id}
              file={file}
              level={level + 1}
              selected={selectedFileId === file.id}
              onSelect={onSelectFile}
              onContextMenu={onContextMenu}
            />
          ))}
        </div>
      )}
    </div>
  );
}

interface FileItemProps {
  file: ContextFile;
  level: number;
  selected: boolean;
  onSelect: (file: ContextFile) => void;
  onContextMenu: (e: React.MouseEvent, file: ContextFile) => void;
}

function FileItem({
  file,
  level,
  selected,
  onSelect,
  onContextMenu,
}: FileItemProps) {
  const handleDragStart = (e: React.DragEvent) => {
    e.dataTransfer.setData("application/x-context-file", file.id);
    e.dataTransfer.effectAllowed = "move";
  };

  return (
    <div
      role="treeitem"
      aria-level={level + 1}
      aria-selected={selected}
      className={`group flex items-center gap-1.5 rounded-lg px-2 py-1.5 cursor-pointer transition-colors text-sm ${
        selected
          ? "bg-accent-muted text-accent-text"
          : "hover:bg-surface-2 text-text-2 hover:text-text-1"
      }`}
      style={{ paddingLeft: `${8 + level * 16}px` }}
      onClick={() => onSelect(file)}
      onContextMenu={(e) => onContextMenu(e, file)}
      draggable
      onDragStart={handleDragStart}
    >
      <GripVertical className="w-3 h-3 text-text-3 opacity-0 group-hover:opacity-50 flex-shrink-0 cursor-grab" />
      <MimeIcon mime={file.mime_type} className="w-4 h-4 flex-shrink-0" />
      <span className="flex-1 truncate">{file.name}</span>
    </div>
  );
}

// ---- Main Page ----------------------------------------------------------------

export function Context() {
  const { toast } = useToast();

  // Tree data
  const [tree, setTree] = useState<ContextTree>({ folders: [], files: [] });
  const [loading, setLoading] = useState(true);

  // Selection & file content
  const [selectedFile, setSelectedFile] = useState<ContextFile | null>(null);
  const [fileContent, setFileContent] = useState<string>("");
  const [contentLoading, setContentLoading] = useState(false);
  const [editedContent, setEditedContent] = useState<string>("");
  const [saving, setSaving] = useState(false);
  const [isDirty, setIsDirty] = useState(false);

  // File rename
  const [renamingFile, setRenamingFile] = useState(false);
  const [renameValue, setRenameValue] = useState("");

  // Folder expand state
  const [expandedFolders, setExpandedFolders] = useState<Set<string>>(
    new Set(),
  );

  // Drag & drop (upload)
  const [isDragging, setIsDragging] = useState(false);
  const dragCounter = useRef(0);

  // Root drop zone for moving files to root
  const [rootDragOver, setRootDragOver] = useState(false);

  // Modals
  const [newFolderOpen, setNewFolderOpen] = useState(false);
  const [newFolderName, setNewFolderName] = useState("");
  const [creatingFolder, setCreatingFolder] = useState(false);
  const [deleteFileOpen, setDeleteFileOpen] = useState(false);
  const [deletingFile, setDeletingFile] = useState(false);
  const [fileToDelete, setFileToDelete] = useState<ContextFile | null>(null);

  // Right-click context menu
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null);

  // Mobile sidebar
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(true);

  // About You mode
  const [aboutYouMode, setAboutYouMode] = useState(false);
  const [aboutYouContent, setAboutYouContent] = useState("");
  const [aboutYouSaved, setAboutYouSaved] = useState("");
  const [aboutYouDirty, setAboutYouDirty] = useState(false);
  const [aboutYouSaving, setAboutYouSaving] = useState(false);
  const [aboutYouLoading, setAboutYouLoading] = useState(false);

  // Upload
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Load tree
  const loadTree = useCallback(async () => {
    try {
      const data = await contextApi.tree();
      setTree(data);
    } catch (e) {
      console.warn("contextOperation failed:", e);
      toast("error", "Failed to load context files");
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    loadTree();
  }, [loadTree]);

  // Auto-expand all folders on first load
  useEffect(() => {
    if (!loading && tree.folders.length > 0) {
      const collectIds = (nodes: ContextTreeNode[]): string[] => {
        return nodes.flatMap((n) => [n.id, ...collectIds(n.children)]);
      };
      setExpandedFolders(new Set(collectIds(tree.folders)));
    }
  }, [loading]); // eslint-disable-line react-hooks/exhaustive-deps

  const toggleFolder = (id: string) => {
    setExpandedFolders((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  // Select a file and load its content
  const selectFile = async (file: ContextFile) => {
    if (isDirty && selectedFile) {
      if (!confirm("You have unsaved changes. Discard them?")) return;
    }
    setAboutYouMode(false);
    setSelectedFile(file);
    setRenamingFile(false);
    setMobileSidebarOpen(false);
    setIsDirty(false);
    setFileContent("");
    setEditedContent("");

    if (isTextMime(file.mime_type)) {
      setContentLoading(true);
      try {
        const result = await contextApi.getFile(file.id);
        const content = result.content ?? "";
        setFileContent(content);
        setEditedContent(content);
      } catch (e) {
        console.warn("contextOperation failed:", e);
        toast("error", "Failed to load file content");
      } finally {
        setContentLoading(false);
      }
    }
  };

  // Update file in tree state
  const updateFileInTree = (updatedFile: ContextFile) => {
    const updateFiles = (files: ContextFile[]) =>
      files.map((f) => (f.id === updatedFile.id ? updatedFile : f));
    const updateNodes = (nodes: ContextTreeNode[]): ContextTreeNode[] =>
      nodes.map((n) => ({
        ...n,
        files: updateFiles(n.files),
        children: updateNodes(n.children),
      }));
    setTree((prev) => ({
      folders: updateNodes(prev.folders),
      files: updateFiles(prev.files),
    }));
  };

  // Save content
  const handleSave = async () => {
    if (!selectedFile) return;
    setSaving(true);
    try {
      await contextApi.updateFile(selectedFile.id, { content: editedContent });
      setFileContent(editedContent);
      setIsDirty(false);
      toast("success", "File saved");
    } catch (e) {
      console.warn("contextOperation failed:", e);
      toast("error", "Failed to save file");
    } finally {
      setSaving(false);
    }
  };

  // Rename file
  const startRename = (file?: ContextFile) => {
    const target = file || selectedFile;
    if (!target) return;
    if (target.id !== selectedFile?.id) {
      selectFile(target);
    }
    setRenameValue(target.name);
    setRenamingFile(true);
  };

  const commitRename = async () => {
    if (!selectedFile || !renameValue.trim()) {
      setRenamingFile(false);
      return;
    }
    try {
      await contextApi.updateFile(selectedFile.id, {
        name: renameValue.trim(),
      });
      const updated = { ...selectedFile, name: renameValue.trim() };
      updateFileInTree(updated);
      setSelectedFile(updated);
      setRenamingFile(false);
      toast("success", "File renamed");
    } catch (e) {
      console.warn("contextOperation failed:", e);
      toast("error", "Failed to rename file");
      setRenamingFile(false);
    }
  };

  // Delete file
  const handleDeleteFile = async () => {
    const target = fileToDelete || selectedFile;
    if (!target) return;
    setDeletingFile(true);
    try {
      await contextApi.deleteFile(target.id);
      const removeFile = (files: ContextFile[]) =>
        files.filter((f) => f.id !== target.id);
      const removeFromNodes = (nodes: ContextTreeNode[]): ContextTreeNode[] =>
        nodes.map((n) => ({
          ...n,
          files: removeFile(n.files),
          children: removeFromNodes(n.children),
        }));
      setTree((prev) => ({
        folders: removeFromNodes(prev.folders),
        files: removeFile(prev.files),
      }));
      if (selectedFile?.id === target.id) {
        setSelectedFile(null);
        setFileContent("");
        setEditedContent("");
      }
      setDeleteFileOpen(false);
      setFileToDelete(null);
      toast("success", "File deleted");
    } catch (e) {
      console.warn("contextOperation failed:", e);
      toast("error", "Failed to delete file");
    } finally {
      setDeletingFile(false);
    }
  };

  // Delete folder
  const handleDeleteFolder = async (id: string, name: string) => {
    if (
      !confirm(
        `Delete folder "${name}" and all its contents? This cannot be undone.`,
      )
    )
      return;
    try {
      await contextApi.deleteFolder(id);
      const removeFolder = (nodes: ContextTreeNode[]): ContextTreeNode[] =>
        nodes
          .filter((n) => n.id !== id)
          .map((n) => ({
            ...n,
            children: removeFolder(n.children),
          }));
      setTree((prev) => ({ ...prev, folders: removeFolder(prev.folders) }));
      toast("success", "Folder deleted");
    } catch (e) {
      console.warn("contextOperation failed:", e);
      toast("error", "Failed to delete folder");
    }
  };

  // Create folder
  const handleCreateFolder = async () => {
    if (!newFolderName.trim()) return;
    setCreatingFolder(true);
    try {
      const folder = await contextApi.createFolder(newFolderName.trim());
      const newNode: ContextTreeNode = { ...folder, children: [], files: [] };
      setTree((prev) => ({ ...prev, folders: [...prev.folders, newNode] }));
      setExpandedFolders((prev) => new Set([...prev, folder.id]));
      setNewFolderOpen(false);
      setNewFolderName("");
      toast("success", "Folder created");
    } catch (e) {
      console.warn("contextOperation failed:", e);
      toast("error", "Failed to create folder");
    } finally {
      setCreatingFolder(false);
    }
  };

  // Upload files
  const handleUpload = async (files: FileList | File[]) => {
    const fileArray = Array.from(files);
    for (const file of fileArray) {
      try {
        const uploaded = await contextApi.uploadFile(file);
        setTree((prev) => ({ ...prev, files: [...prev.files, uploaded] }));
        toast("success", `Uploaded ${file.name}`);
      } catch (e) {
        console.warn("contextOperation failed:", e);
        toast("error", `Failed to upload ${file.name}`);
      }
    }
  };

  // Drag & drop handlers (upload from desktop)
  const handleDragEnter = (e: React.DragEvent) => {
    e.preventDefault();
    dragCounter.current++;
    if (e.dataTransfer.types.includes("Files")) {
      setIsDragging(true);
    }
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    dragCounter.current--;
    if (dragCounter.current === 0) {
      setIsDragging(false);
    }
  };

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    dragCounter.current = 0;
    setIsDragging(false);
    if (e.dataTransfer.files.length > 0) {
      handleUpload(e.dataTransfer.files);
    }
  };

  // Move file between folders
  const handleFileDrop = async (fileId: string, folderId: string) => {
    try {
      await contextApi.moveFile(fileId, folderId);
      await loadTree();
      toast("success", "File moved");
    } catch (e) {
      console.warn("contextOperation failed:", e);
      toast("error", "Failed to move file");
    }
  };

  // Move file to root
  const handleRootDrop = async (e: React.DragEvent) => {
    e.preventDefault();
    setRootDragOver(false);
    const fileId = e.dataTransfer.getData("application/x-context-file");
    if (fileId) {
      try {
        await contextApi.moveFile(fileId, null);
        await loadTree();
        toast("success", "File moved to root");
      } catch (e) {
        console.warn("contextOperation failed:", e);
        toast("error", "Failed to move file");
      }
    }
  };

  // Right-click context menu
  const handleFileContextMenu = (e: React.MouseEvent, file: ContextFile) => {
    e.preventDefault();
    setContextMenu({ x: e.clientX, y: e.clientY, file });
  };

  const handleContextMenuRename = (file: ContextFile) => {
    selectFile(file).then(() => {
      setRenameValue(file.name);
      setRenamingFile(true);
    });
  };

  const handleContextMenuDelete = (file: ContextFile) => {
    setFileToDelete(file);
    setDeleteFileOpen(true);
  };

  // About You
  const openAboutYou = async () => {
    if (isDirty && selectedFile) {
      if (!confirm("You have unsaved changes. Discard them?")) return;
    }
    setAboutYouMode(true);
    setSelectedFile(null);
    setMobileSidebarOpen(false);
    setIsDirty(false);
    setAboutYouLoading(true);
    try {
      const result = await contextApi.getAboutYou();
      setAboutYouContent(result.content || "");
      setAboutYouSaved(result.content || "");
      setAboutYouDirty(false);
    } catch (e) {
      console.warn("contextOperation failed:", e);
      toast("error", "Failed to load About You");
    } finally {
      setAboutYouLoading(false);
    }
  };

  const saveAboutYou = async () => {
    setAboutYouSaving(true);
    try {
      await contextApi.updateAboutYou(aboutYouContent);
      setAboutYouSaved(aboutYouContent);
      setAboutYouDirty(false);
      toast("success", "About You saved");
    } catch (e) {
      console.warn("contextOperation failed:", e);
      toast("error", "Failed to save About You");
    } finally {
      setAboutYouSaving(false);
    }
  };

  const totalFiles =
    tree.files.length +
    (function count(nodes: ContextTreeNode[]): number {
      return nodes.reduce(
        (acc, n) => acc + n.files.length + count(n.children),
        0,
      );
    })(tree.folders);

  return (
    <div
      className="flex flex-col h-full relative"
      onDragEnter={handleDragEnter}
      onDragLeave={handleDragLeave}
      onDragOver={handleDragOver}
      onDrop={handleDrop}
    >
      <Header title="Context" />

      {/* Drag overlay (upload from desktop) */}
      {isDragging && (
        <div className="absolute inset-0 z-50 flex items-center justify-center pointer-events-none">
          <div className="absolute inset-4 rounded-xl border-2 border-dashed border-accent-primary bg-accent-muted/60 backdrop-blur-sm flex items-center justify-center">
            <div className="text-center">
              <Upload className="w-10 h-10 text-accent-primary mx-auto mb-2" />
              <p className="text-base font-semibold text-accent-text">
                Drop files to upload
              </p>
            </div>
          </div>
        </div>
      )}

      <div className="flex flex-1 min-h-0 relative">
        {/* ---- Mobile sidebar overlay ---------------------------------------- */}
        {mobileSidebarOpen && (
          <div
            className="absolute inset-0 z-20 bg-black/50 md:hidden"
            onClick={() => setMobileSidebarOpen(false)}
          />
        )}

        {/* ---- Left sidebar -------------------------------------------------- */}
        <aside
          className={`${mobileSidebarOpen ? "translate-x-0" : "-translate-x-full"} md:translate-x-0 absolute md:relative z-30 md:z-auto w-[280px] h-full flex flex-col border-r border-border-0 bg-surface-1 flex-shrink-0 overflow-hidden transition-transform duration-200 ease-out`}
        >
          <div className="flex-1 overflow-y-auto py-2 px-2 space-y-0.5">
            {/* About You button — always visible at top */}
            <div className="mb-1">
              <Button
                variant={aboutYouMode ? "primary" : "secondary"}
                size="sm"
                onClick={openAboutYou}
                icon={<UserPen className="w-4 h-4" />}
                className="w-full"
              >
                About You
              </Button>
            </div>

            <div className="mx-2 mb-1 border-b border-border-0" />

            {loading ? (
              <div className="flex items-center justify-center h-24 text-text-3 text-sm">
                Loading...
              </div>
            ) : totalFiles === 0 && tree.folders.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-40 text-center px-4 gap-2">
                <File className="w-8 h-8 text-text-3" />
                <p className="text-sm text-text-2 font-medium">
                  No context files
                </p>
                <p className="text-xs text-text-3">
                  Upload files or create a folder.
                </p>
              </div>
            ) : (
              <>
                <div role="tree" aria-label="Context files">
                  {/* Folder tree */}
                  {tree.folders.map((node) => (
                    <TreeNodeItem
                      key={node.id}
                      node={node}
                      level={0}
                      expanded={expandedFolders}
                      selectedFileId={selectedFile?.id ?? null}
                      onToggleFolder={toggleFolder}
                      onSelectFile={selectFile}
                      onDeleteFolder={handleDeleteFolder}
                      onFileDrop={handleFileDrop}
                      onContextMenu={handleFileContextMenu}
                    />
                  ))}

                  {/* Root-level files */}
                  {tree.files.length > 0 && (
                    <div role="group">
                      {tree.folders.length > 0 && (
                        <div className="mx-2 my-1 border-b border-border-0" />
                      )}
                      {tree.files.map((file) => (
                        <FileItem
                          key={file.id}
                          file={file}
                          level={0}
                          selected={selectedFile?.id === file.id}
                          onSelect={selectFile}
                          onContextMenu={handleFileContextMenu}
                        />
                      ))}
                    </div>
                  )}
                </div>

                {/* Root drop zone — move files to root */}
                {tree.folders.length > 0 && (
                  <div
                    className={`mx-2 mt-2 rounded-lg border-2 border-dashed py-3 text-center text-xs transition-colors ${
                      rootDragOver
                        ? "border-accent-primary bg-accent-muted/30 text-accent-text"
                        : "border-border-0 text-text-3"
                    }`}
                    onDragOver={(e) => {
                      if (
                        e.dataTransfer.types.includes(
                          "application/x-context-file",
                        )
                      ) {
                        e.preventDefault();
                        e.dataTransfer.dropEffect = "move";
                        setRootDragOver(true);
                      }
                    }}
                    onDragLeave={() => setRootDragOver(false)}
                    onDrop={handleRootDrop}
                  >
                    Drop here for root
                  </div>
                )}
              </>
            )}
          </div>

          {/* Sidebar footer */}
          <div className="border-t border-border-0 p-2 flex gap-2">
            <button
              onClick={() => setNewFolderOpen(true)}
              className="flex-1 flex items-center justify-center gap-1.5 px-2 py-1.5 rounded-lg text-xs font-medium text-text-2 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer border border-border-0"
            >
              <FolderPlus className="w-3.5 h-3.5" />
              New Folder
            </button>
            <button
              onClick={() => fileInputRef.current?.click()}
              className="flex-1 flex items-center justify-center gap-1.5 px-2 py-1.5 rounded-lg text-xs font-medium text-text-2 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer border border-border-0"
            >
              <Upload className="w-3.5 h-3.5" />
              Upload
            </button>
            <input
              ref={fileInputRef}
              type="file"
              multiple
              className="hidden"
              onChange={(e) => {
                if (e.target.files) handleUpload(e.target.files);
                e.target.value = "";
              }}
            />
          </div>
        </aside>

        {/* ---- Right panel --------------------------------------------------- */}
        <main className="flex-1 flex flex-col min-w-0 bg-surface-0 overflow-hidden">
          {aboutYouMode ? (
            <>
              {/* About You header */}
              <div className="flex items-center justify-between px-3 md:px-5 py-3 border-b border-border-0 bg-surface-1/50 backdrop-blur-sm flex-shrink-0 gap-2">
                <div className="flex items-center gap-2 md:gap-3 min-w-0 flex-1">
                  <button
                    onClick={() => setMobileSidebarOpen(true)}
                    className="md:hidden p-1.5 -ml-1 rounded-lg text-text-2 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer flex-shrink-0"
                  >
                    <PanelLeft className="w-4 h-4" />
                  </button>
                  <UserPen className="w-5 h-5 text-accent-primary flex-shrink-0" />
                  <span className="text-sm font-semibold text-text-0">
                    About You
                  </span>
                </div>
                <Button
                  variant={aboutYouDirty ? "primary" : "ghost"}
                  size="sm"
                  loading={aboutYouSaving}
                  disabled={!aboutYouDirty}
                  onClick={saveAboutYou}
                  icon={<Save className="w-3.5 h-3.5" />}
                >
                  Save
                </Button>
              </div>

              {/* About You body */}
              <div className="flex-1 overflow-hidden flex flex-col">
                <div className="px-3 md:px-5 pt-3 md:pt-4 pb-2">
                  <p className="text-xs text-text-3">
                    Write about yourself, your preferences, and anything you
                    want your agents to know. This is included in every chat as
                    context.
                  </p>
                </div>
                {aboutYouLoading ? (
                  <div className="flex-1 flex items-center justify-center text-text-3 text-sm">
                    Loading...
                  </div>
                ) : (
                  <div className="flex-1 px-3 md:px-5 pb-3 md:pb-5">
                    <textarea
                      className="w-full h-full resize-none bg-surface-1 text-text-1 text-sm font-mono p-3 md:p-4 outline-none leading-relaxed rounded-lg border border-border-0"
                      value={aboutYouContent}
                      onChange={(e) => {
                        setAboutYouContent(e.target.value);
                        setAboutYouDirty(e.target.value !== aboutYouSaved);
                      }}
                      placeholder="Tell your agents about yourself..."
                      spellCheck={false}
                    />
                  </div>
                )}
              </div>
            </>
          ) : !selectedFile ? (
            <div className="flex-1 flex flex-col items-center justify-center gap-4 text-center px-8">
              <button
                onClick={() => setMobileSidebarOpen(true)}
                className="md:hidden p-3 rounded-2xl bg-surface-2 text-text-2 hover:bg-surface-3 transition-colors cursor-pointer"
              >
                <PanelLeft className="w-6 h-6" />
              </button>
              <div className="hidden md:flex w-16 h-16 rounded-2xl bg-surface-2 items-center justify-center">
                <File className="w-8 h-8 text-text-3" />
              </div>
              <div>
                <p className="text-base font-semibold text-text-1 mb-1">
                  Select a file
                </p>
                <p className="text-sm text-text-3">
                  Choose a file from the sidebar to view or edit it.
                </p>
              </div>
              <Button
                variant="secondary"
                size="sm"
                icon={<Upload className="w-4 h-4" />}
                onClick={() => fileInputRef.current?.click()}
              >
                Upload File
              </Button>
            </div>
          ) : (
            <>
              {/* File header */}
              <div className="flex items-center justify-between px-3 md:px-5 py-3 border-b border-border-0 bg-surface-1/50 backdrop-blur-sm flex-shrink-0 gap-2">
                <div className="flex items-center gap-2 md:gap-3 min-w-0 flex-1">
                  <button
                    onClick={() => setMobileSidebarOpen(true)}
                    className="md:hidden p-1.5 -ml-1 rounded-lg text-text-2 hover:text-text-1 hover:bg-surface-2 transition-colors cursor-pointer flex-shrink-0"
                  >
                    <PanelLeft className="w-4 h-4" />
                  </button>
                  {renamingFile ? (
                    <input
                      autoFocus
                      value={renameValue}
                      onChange={(e) => setRenameValue(e.target.value)}
                      onBlur={commitRename}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") commitRename();
                        if (e.key === "Escape") setRenamingFile(false);
                      }}
                      className="text-sm font-semibold text-text-0 bg-surface-2 border border-accent-primary rounded-md px-2 py-0.5 outline-none min-w-0 flex-1"
                    />
                  ) : (
                    <button
                      onClick={() => startRename()}
                      className="text-sm font-semibold text-text-0 hover:text-accent-text truncate cursor-pointer"
                      title="Click to rename"
                    >
                      {selectedFile.name}
                    </button>
                  )}
                  <span className="hidden sm:inline text-xs px-1.5 py-0.5 rounded bg-surface-3 text-text-3 flex-shrink-0">
                    {selectedFile.mime_type}
                  </span>
                  <span className="hidden sm:inline text-xs text-text-3 flex-shrink-0">
                    {formatBytes(selectedFile.size_bytes)}
                  </span>
                </div>

                <div className="flex items-center gap-1.5 md:gap-2 flex-shrink-0">
                  {/* Save button (text files only) */}
                  {isTextMime(selectedFile.mime_type) && (
                    <Button
                      variant={isDirty ? "primary" : "ghost"}
                      size="sm"
                      loading={saving}
                      disabled={!isDirty}
                      onClick={handleSave}
                      icon={<Save className="w-3.5 h-3.5" />}
                    >
                      <span className="hidden sm:inline">Save</span>
                    </Button>
                  )}

                  {/* Delete */}
                  <button
                    onClick={() => {
                      setFileToDelete(selectedFile);
                      setDeleteFileOpen(true);
                    }}
                    className="p-1.5 rounded-lg text-text-3 hover:text-red-400 hover:bg-red-500/10 transition-colors cursor-pointer"
                    title="Delete file"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>

              {/* File body */}
              <div className="flex-1 overflow-hidden flex flex-col">
                {contentLoading ? (
                  <div className="flex-1 flex items-center justify-center text-text-3 text-sm">
                    Loading content...
                  </div>
                ) : isTextMime(selectedFile.mime_type) ? (
                  <div className="flex-1 px-3 md:px-5 pb-3 md:pb-5 pt-3">
                    <textarea
                      className="w-full h-full resize-none bg-surface-1 text-text-1 text-sm font-mono p-3 md:p-4 outline-none leading-relaxed rounded-lg border border-border-0"
                      value={editedContent}
                      onChange={(e) => {
                        setEditedContent(e.target.value);
                        setIsDirty(e.target.value !== fileContent);
                      }}
                      placeholder="File is empty..."
                      spellCheck={false}
                    />
                  </div>
                ) : isImageMime(selectedFile.mime_type) ? (
                  <div className="flex-1 flex items-center justify-center p-4 md:p-8 bg-surface-0 overflow-auto">
                    <img
                      src={contextApi.rawFileUrl(selectedFile.id)}
                      alt={selectedFile.name}
                      className="max-w-full max-h-full object-contain rounded-lg shadow-lg"
                    />
                  </div>
                ) : (
                  <div className="flex-1 flex flex-col items-center justify-center gap-4 text-center p-4 md:p-8">
                    <div className="w-16 h-16 rounded-2xl bg-surface-2 flex items-center justify-center">
                      <Database className="w-8 h-8 text-text-3" />
                    </div>
                    <div>
                      <p className="text-sm font-semibold text-text-1 mb-1">
                        {selectedFile.name}
                      </p>
                      <p className="text-xs text-text-3 mb-1">
                        {selectedFile.mime_type}
                      </p>
                      <p className="text-xs text-text-3">
                        {formatBytes(selectedFile.size_bytes)}
                      </p>
                    </div>
                    <a
                      href={contextApi.rawFileUrl(selectedFile.id)}
                      download={selectedFile.filename}
                      className="text-xs text-accent-text hover:underline"
                    >
                      Download file
                    </a>
                  </div>
                )}
              </div>
            </>
          )}
        </main>
      </div>

      {/* ---- Right-click Context Menu ---------------------------------------- */}
      {contextMenu && (
        <ContextMenu
          menu={contextMenu}
          onRename={handleContextMenuRename}
          onDelete={handleContextMenuDelete}
          onClose={() => setContextMenu(null)}
        />
      )}

      {/* ---- New Folder Modal ----------------------------------------------- */}
      <Modal
        open={newFolderOpen}
        onClose={() => {
          setNewFolderOpen(false);
          setNewFolderName("");
        }}
        title="New Folder"
        size="sm"
      >
        <div className="space-y-4">
          <div>
            <label className="block text-xs font-medium text-text-2 mb-1.5">
              Folder name
            </label>
            <input
              autoFocus
              value={newFolderName}
              onChange={(e) => setNewFolderName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleCreateFolder();
                if (e.key === "Escape") {
                  setNewFolderOpen(false);
                  setNewFolderName("");
                }
              }}
              className="w-full text-sm bg-surface-2 border border-border-1 rounded-lg px-3 py-2 text-text-0 outline-none focus:border-accent-primary transition-colors"
              placeholder="e.g. Personal, Work, Projects"
            />
          </div>
          <div className="flex justify-end gap-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setNewFolderOpen(false);
                setNewFolderName("");
              }}
            >
              Cancel
            </Button>
            <Button
              size="sm"
              loading={creatingFolder}
              disabled={!newFolderName.trim()}
              onClick={handleCreateFolder}
              icon={<FolderPlus className="w-3.5 h-3.5" />}
            >
              Create Folder
            </Button>
          </div>
        </div>
      </Modal>

      {/* ---- Delete File Modal ---------------------------------------------- */}
      <Modal
        open={deleteFileOpen}
        onClose={() => {
          setDeleteFileOpen(false);
          setFileToDelete(null);
        }}
        title="Delete File"
        size="sm"
      >
        <div className="space-y-4">
          <p className="text-sm text-text-2">
            Are you sure you want to delete{" "}
            <span className="font-semibold text-text-0">
              {(fileToDelete || selectedFile)?.name}
            </span>
            ? This cannot be undone.
          </p>
          <div className="flex justify-end gap-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setDeleteFileOpen(false);
                setFileToDelete(null);
              }}
            >
              Cancel
            </Button>
            <Button
              variant="danger"
              size="sm"
              loading={deletingFile}
              onClick={handleDeleteFile}
              icon={<Trash2 className="w-3.5 h-3.5" />}
            >
              Delete
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
