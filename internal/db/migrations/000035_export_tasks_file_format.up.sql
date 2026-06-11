-- Add file_format column to export_tasks to track CSV vs Excel exports
ALTER TABLE export_tasks ADD COLUMN file_format TEXT NOT NULL DEFAULT 'csv';
