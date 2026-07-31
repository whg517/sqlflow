package authz

import (
	"errors"
	"testing"
)

func TestNormalizeTuple(t *testing.T) {
	tests := []struct {
		name                      string
		sub, dom, obj, act        string
		wantSub, wantDom, wantAct string
		wantErr                   bool
	}{
		{
			name: "role with canonical datasource",
			sub:  "Developer", dom: "ds_42", obj: "orders", act: "SELECT",
			wantSub: "developer", wantDom: "ds_42", wantAct: "select",
		},
		{
			name: "bare datasource id is canonicalized",
			sub:  "dba", dom: "7", obj: "*", act: "ddl",
			wantSub: "dba", wantDom: "ds_7", wantAct: "ddl",
		},
		{
			name: "user subject and wildcard",
			sub:  "user:9", dom: "*", obj: "*", act: "export",
			wantSub: "user:9", wantDom: "*", wantAct: "export",
		},
		{name: "invalid user", sub: "user:0", dom: "ds_1", obj: "t", act: "select", wantErr: true},
		{name: "invalid domain", sub: "developer", dom: "prod", obj: "t", act: "select", wantErr: true},
		{name: "invalid action", sub: "developer", dom: "ds_1", obj: "t", act: "execute", wantErr: true},
		{name: "empty object", sub: "developer", dom: "ds_1", obj: "", act: "select", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub, dom, _, act, err := NormalizeTuple(tt.sub, tt.dom, tt.obj, tt.act)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidTuple) {
					t.Fatalf("NormalizeTuple() error = %v, want ErrInvalidTuple", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeTuple() error: %v", err)
			}
			if sub != tt.wantSub || dom != tt.wantDom || act != tt.wantAct {
				t.Fatalf("NormalizeTuple() = (%q, %q, %q), want (%q, %q, %q)",
					sub, dom, act, tt.wantSub, tt.wantDom, tt.wantAct)
			}
		})
	}
}

func TestTupleConstructors(t *testing.T) {
	if got := DatasourceDomain(12); got != "ds_12" {
		t.Fatalf("DatasourceDomain(12) = %q", got)
	}
	if got := UserSubject(34); got != "user:34" {
		t.Fatalf("UserSubject(34) = %q", got)
	}
}
