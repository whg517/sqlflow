package handler

import (
	"log"

	"github.com/labstack/echo/v4"
	"github.com/whg517/sqlflow/internal/resp"
	"github.com/whg517/sqlflow/internal/service"
)

// BackupHandler handles backup-related API requests.
type BackupHandler struct {
	backupSvc *service.BackupService
}

// NewBackupHandler creates a new BackupHandler.
func NewBackupHandler(backupSvc *service.BackupService) *BackupHandler {
	return &BackupHandler{backupSvc: backupSvc}
}

// TriggerBackup handles POST /api/backups — manually trigger an immediate backup.
//
// @Summary 手动触发备份
// @Description 管理员手动触发一次数据库备份
// @Tags 备份
// @Produce json
// @Security BearerAuth
// @Success 200 {object} resp.SuccessResponse "备份已创建"
// @Failure 500 {object} resp.ErrorResponse "备份失败"
// @Router /backups [post]
func (h *BackupHandler) TriggerBackup(c echo.Context) error {
	if err := h.backupSvc.RunBackup(); err != nil {
		log.Printf("TriggerBackup failed: %v", err)
		return resp.InternalError(c, "备份失败: "+err.Error())
	}
	return resp.OK(c, map[string]string{"message": "备份已创建"})
}

// ListBackups handles GET /api/backups — list all existing backup files.
//
// @Summary 获取备份列表
// @Description 管理员获取所有备份文件列表
// @Tags 备份
// @Produce json
// @Security BearerAuth
// @Success 200 {object} resp.SuccessResponse "成功"
// @Failure 500 {object} resp.ErrorResponse "获取备份列表失败"
// @Router /backups [get]
func (h *BackupHandler) ListBackups(c echo.Context) error {
	backups, err := h.backupSvc.ListBackups()
	if err != nil {
		log.Printf("ListBackups failed: %v", err)
		return resp.InternalError(c, "获取备份列表失败")
	}
	return resp.OK(c, backups)
}

// DeleteBackup handles DELETE /api/backups/:filename — delete a specific backup file.
//
// @Summary 删除备份
// @Description 管理员删除指定的备份文件
// @Tags 备份
// @Produce json
// @Security BearerAuth
// @Param filename path string true "备份文件名"
// @Success 200 {object} resp.SuccessResponse "备份已删除"
// @Failure 400 {object} resp.ErrorResponse "缺少备份文件名"
// @Router /backups/{filename} [delete]
func (h *BackupHandler) DeleteBackup(c echo.Context) error {
	filename := c.Param("filename")
	if filename == "" {
		return resp.BadRequest(c, "缺少备份文件名")
	}
	if err := h.backupSvc.DeleteBackup(filename); err != nil {
		log.Printf("DeleteBackup failed: %v", err)
		return resp.BadRequest(c, err.Error())
	}
	return resp.OK(c, map[string]string{"message": "备份已删除"})
}

// DownloadBackup handles GET /api/backups/:filename/download — download a backup file.
//
// @Summary 下载备份
// @Description 管理员下载指定的备份文件
// @Tags 备份
// @Produce octet-stream
// @Security BearerAuth
// @Param filename path string true "备份文件名"
// @Success 200 {file} file "备份文件"
// @Failure 400 {object} resp.ErrorResponse "缺少备份文件名"
// @Failure 404 {object} resp.ErrorResponse "备份文件不存在"
// @Router /backups/{filename}/download [get]
func (h *BackupHandler) DownloadBackup(c echo.Context) error {
	filename := c.Param("filename")
	if filename == "" {
		return resp.BadRequest(c, "缺少备份文件名")
	}

	backups, err := h.backupSvc.ListBackups()
	if err != nil {
		return resp.InternalError(c, "获取备份列表失败")
	}

	// Find the requested file
	var found bool
	var filepath string
	for _, b := range backups {
		if b.Filename == filename {
			found = true
			filepath = b.Filepath
			break
		}
	}
	if !found {
		return resp.NotFound(c, "备份文件不存在")
	}

	return c.File(filepath)
}
