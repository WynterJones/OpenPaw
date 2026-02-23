package handlers

import (
	"github.com/openpaw/openpaw/internal/backup"
)

func testBackupConnection(repoURL, authToken, authMethod string) error {
	return backup.TestConnection(repoURL, authToken, authMethod)
}

func detectGitMethodFromHandler() backup.GitMethod {
	return backup.DetectGitMethod()
}
