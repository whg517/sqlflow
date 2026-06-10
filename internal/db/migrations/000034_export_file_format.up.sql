-- SF-FEAT0054: Add file_format column to export_tasks
ALTER TABLE export_tasks ADD COLUMN file_format TEXT NOT NULL DEFAULT 'csv';
