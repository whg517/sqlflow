package service

import (
	"testing"
)

func TestExportRowLimit(t *testing.T) {
	if exportRowLimit != 10000 {
		t.Errorf("exportRowLimit = %d, want 10000", exportRowLimit)
	}
}

func TestErrExportRowLimit(t *testing.T) {
	want := "导出数据超过10000行上限，请添加 LIMIT 条件缩小范围"
	if ErrExportRowLimit.Error() != want {
		t.Errorf("ErrExportRowLimit = %q, want %q", ErrExportRowLimit.Error(), want)
	}
}

func TestExportConstants(t *testing.T) {
	tests := []struct {
		name string
		got  int
		want int
	}{
		{"defaultRowLimit", defaultRowLimit, 1000},
		{"exportRowLimit", exportRowLimit, 10000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
			}
		})
	}
}
