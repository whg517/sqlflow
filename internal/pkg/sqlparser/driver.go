package sqlparser

import (
	"fmt"
	"io"

	"github.com/pingcap/parser/ast"
	"github.com/pingcap/parser/format"
	"github.com/pingcap/parser/types"
)

func init() {
	// Register minimal parser driver for pingcap/parser.
	// These functions are required by parser.New() to construct AST value nodes.
	ast.NewValueExpr = func(val interface{}) ast.ValueExpr {
		return &simpleValueExpr{TexprNode: ast.TexprNode{}, val: val}
	}
	ast.NewParamMarkerExpr = func(offset int) ast.ParamMarkerExpr {
		return &simpleParamMarkerExpr{
			simpleValueExpr: simpleValueExpr{TexprNode: ast.TexprNode{}},
			offset:          offset,
		}
	}
	ast.NewHexLiteral = func(s string) (interface{}, error) { return s, nil }
	ast.NewBitLiteral = func(s string) (interface{}, error) { return s, nil }
}

type simpleValueExpr struct {
	ast.TexprNode
	val        interface{}
	projOffset int
}

func (e *simpleValueExpr) SetValue(val interface{})       { e.val = val }
func (e *simpleValueExpr) GetValue() interface{}          { return e.val }
func (e *simpleValueExpr) GetDatumString() string         { return "" }
func (e *simpleValueExpr) GetString() string              { return "" }
func (e *simpleValueExpr) GetProjectionOffset() int       { return e.projOffset }
func (e *simpleValueExpr) SetProjectionOffset(offset int) { e.projOffset = offset }

// Format implements ast.ExprNode.
func (e *simpleValueExpr) Format(w io.Writer) {
	_, _ = fmt.Fprintf(w, "%v", e.val)
}

// Restore implements ast.Node.
func (e *simpleValueExpr) Restore(ctx *format.RestoreCtx) error {
	ctx.WritePlain(fmt.Sprintf("%v", e.val))
	return nil
}

// Accept implements ast.Node.
func (e *simpleValueExpr) Accept(v ast.Visitor) (ast.Node, bool) {
	return v.Enter(e)
}

type simpleParamMarkerExpr struct {
	simpleValueExpr
	offset int
	order  int
}

func (e *simpleParamMarkerExpr) SetOrder(order int) { e.order = order }

// Accept implements ast.Node.
func (e *simpleParamMarkerExpr) Accept(v ast.Visitor) (ast.Node, bool) {
	return v.Enter(e)
}

// Ensure types implement interfaces.
var (
	_ ast.ValueExpr       = (*simpleValueExpr)(nil)
	_ ast.ParamMarkerExpr = (*simpleParamMarkerExpr)(nil)
)

// Ensure types.FieldType is available (needed by TexprNode).
var _ types.FieldType
