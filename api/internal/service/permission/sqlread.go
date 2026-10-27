package permission

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/AfterShip/clickhouse-sql-parser/parser"
	"github.com/pkg/errors"

	"github.com/clickvisual/clickvisual/api/internal/invoker"
	db2 "github.com/clickvisual/clickvisual/api/internal/pkg/model/db"
	"github.com/clickvisual/clickvisual/api/internal/pkg/model/view"
	"github.com/clickvisual/clickvisual/api/internal/service/permission/pmsplugin"
)

// sqlTableRef is a fully qualified ClickHouse table referenced by a query.
type sqlTableRef struct {
	Database string
	Table    string
}

func (r sqlTableRef) String() string {
	return "`" + r.Database + "`.`" + r.Table + "`"
}

// CheckSQLReadPermission authorizes one raw SQL statement issued on instance iid by uid.
//
// Every table the statement references is authorized on its own with the BaseTable domain,
// so table-scoped grants and database-scoped grants both work. Anything the parser cannot be
// trusted to resolve is denied instead of falling back to a coarser check: a fallback would
// leave "write the query in a shape the checker does not understand" as a way through.
func CheckSQLReadPermission(uid, iid int, rawSQL string) error {
	refs, err := ExtractSQLTableRefs(rawSQL)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		tid, err := resolveSQLTableID(iid, ref)
		if err != nil {
			return err
		}
		if err = Manager.CheckNormalPermission(view.ReqPermission{
			UserId:      uid,
			ObjectType:  pmsplugin.PrefixInstance,
			ObjectIdx:   strconv.Itoa(iid),
			SubResource: pmsplugin.Log,
			Acts:        []string{pmsplugin.ActView},
			DomainType:  pmsplugin.PrefixTable,
			DomainId:    strconv.Itoa(tid),
		}); err != nil {
			return errors.Wrapf(err, "缺少表 %s 的读取权限", ref)
		}
	}
	return nil
}

// ExtractSQLTableRefs returns the distinct tables referenced by the single SELECT statement
// in rawSQL, including the ones inside subqueries, joins, WITH clauses and IN (SELECT ...).
func ExtractSQLTableRefs(rawSQL string) ([]sqlTableRef, error) {
	stmts, err := parser.NewParser(rawSQL).ParseStmts()
	if err != nil {
		return nil, fmt.Errorf("无法解析该查询语句，已拒绝执行: %w", err)
	}
	if len(stmts) != 1 {
		return nil, fmt.Errorf("一次只允许执行一条语句. ")
	}
	if _, isSelect := stmts[0].(*parser.SelectQuery); !isSelect {
		return nil, fmt.Errorf("只允许执行 SELECT 语句. ")
	}
	// The CTE names must be known before the references are collected: a reference to a CTE
	// is reported as a plain table, and the traversal does not guarantee any order.
	cteNames := collectCTENames(stmts[0])
	var (
		refs        []sqlTableRef
		seen        = make(map[string]struct{})
		unsupported string
	)
	// Returning false aborts the whole traversal, which is what is wanted here: as soon as
	// one object cannot be authorized the statement is denied and the partial list is unused.
	_ = parser.Walk(stmts[0], func(node parser.Expr) bool {
		switch item := node.(type) {
		case *parser.TableFunctionExpr:
			unsupported = "表函数(numbers/merge/file/url/cluster 等)"
			return false
		case *parser.TableIdentifier:
			if item.Database != nil {
				key := item.Database.Name + "." + item.Table.Name
				if _, duplicated := seen[key]; !duplicated {
					seen[key] = struct{}{}
					refs = append(refs, sqlTableRef{Database: item.Database.Name, Table: item.Table.Name})
				}
				return true
			}
			if _, isCTE := cteNames[strings.ToLower(item.Table.Name)]; isCTE {
				return true
			}
			// An unqualified name is resolved by ClickHouse against the connection's default
			// database, which is not necessarily the table this checker would find.
			unsupported = fmt.Sprintf("未限定的表名 %q，请写成 `库`.`表` 的形式", item.Table.Name)
			return false
		}
		return true
	})
	if unsupported != "" {
		return nil, fmt.Errorf("查询包含无法鉴权的对象：%s. ", unsupported)
	}
	if len(refs) == 0 {
		return nil, fmt.Errorf("查询未引用任何数据表，已拒绝执行. ")
	}
	return refs, nil
}

func collectCTENames(stmt parser.Expr) map[string]struct{} {
	names := make(map[string]struct{})
	_ = parser.Walk(stmt, func(node parser.Expr) bool {
		with, isWith := node.(*parser.WithClause)
		if !isWith {
			return true
		}
		for _, cte := range with.CTEs {
			// CTEStmt.Expr holds the name and .Alias the body, despite the field names.
			if ident, isIdent := cte.Expr.(*parser.Ident); isIdent {
				names[strings.ToLower(ident.Name)] = struct{}{}
			}
		}
		return true
	})
	return names
}

// resolveSQLTableID maps a qualified table name of the query to the metadata table of the
// given instance. Tables that ClickVisual does not manage are denied: no permission record
// exists that could authorize them.
func resolveSQLTableID(iid int, ref sqlTableRef) (int, error) {
	database, err := db2.DatabaseInfoX(invoker.Db, map[string]interface{}{
		"iid":  iid,
		"name": ref.Database,
	})
	if err != nil {
		return 0, errors.Wrapf(err, "query database %s of instance %d failed", ref.Database, iid)
	}
	if database.ID == 0 {
		return 0, fmt.Errorf("库 `%s` 不属于实例 %d 或未在 ClickVisual 中登记. ", ref.Database, iid)
	}
	table, err := db2.TableInfoX(invoker.Db, map[string]interface{}{
		"did":  database.ID,
		"name": ref.Table,
	})
	if err != nil {
		return 0, errors.Wrapf(err, "query table %s of database %d failed", ref.Table, database.ID)
	}
	if table.ID == 0 {
		return 0, fmt.Errorf("表 %s 未在 ClickVisual 中登记. ", ref)
	}
	return table.ID, nil
}
