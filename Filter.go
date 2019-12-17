package main

import (
	"strconv"
	"strings"
)

func operatorToSQL(operator uint8) string {
	switch operator {
	case 1:
		return "="
	case 2:
		return "<"
	case 3:
		return ">"
	case 4:
		return "!="
	case 5:
		return "IN"
	case 6:
		return "NOT IN"
	default:
		return ""
	}
}

func destToSQL(dest uint8) string {
	switch dest {
	case 1:
		return "reportCount"
	case 2:
		return "isProxy"
	case 3:
		return "validated"
	case 4:
		return "lastReport"
	case 5:
		return "firstReport"
	case 6:
		return "domain"
	case 7:
		return "hostname"
	case 8:
		return "type"
	case 9:
		return "KnownAbuser"
	case 10:
		return "KnownHacker"
	case 11:
		return "SELECT 1 FROM IPports WHERE count>0 AND ip=BlockedIP.pk_id AND port"
	case 12:
		return ""
	default:
		return ""
	}
}

func filterPartToSQL(part FilterPart) string {
	operator := operatorToSQL(part.Operator)
	column := destToSQL(part.Dest)
	if len(operator) == 0 {
		return ""
	}
	if len(column) == 0 {
		return ""
	}
	val := part.Val
	if part.Dest == 2 || part.Dest == 3 || part.Dest == 9 || part.Dest == 10 {
		if val == "true" {
			val = "1"
		} else if val == "false" {
			val = "0"
		} else {
			LogError("wrog bool value \"" + val + "\" for part: " + strconv.FormatUint(uint64(part.ID), 10))
			return ""
		}
	}
	if part.Operator == 5 || part.Operator == 6 {
		if part.Dest == 6 || part.Dest == 7 {
			d := strings.Split(part.Val, ",")
			e := ""
			for _, s := range d {
				e += "'" + s + "',"
			}
			e = e[:len(e)-1]
			val = e
		} else if part.Dest != 8 && part.Dest != 11 {
			return ""
		}
		val = "(" + val + ")"
	} else if !isNumeric(val) {
		val = "'" + val + "'"
	}
	return column + " " + operator + " " + val
}

func isNumeric(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

func getFilterSQL(filter Filter) (string, error) {
	sqlWhere := ""
	for _, row := range filter.Rows {
		matchRow, err := getFilterRowSQL(row)
		if err != nil {
			return "", err
		}
		sqlWhere += "(" + matchRow + ") OR"
	}
	if strings.HasSuffix(sqlWhere, "OR") {
		sqlWhere = sqlWhere[:len(sqlWhere)-3]
	}
	return sqlWhere, nil
}

func getFilterRowSQL(rowData FilterRow) (string, error) {
	rowSQL := ""
	for _, row := range rowData.Row {
		part := filterPartToSQL(*row)
		if len(part) > 0 {
			if len(rowData.Row) > 1 {
				part = "(" + part + ")"
			}
			rowSQL += part + " AND"
		}
	}
	if strings.HasSuffix(rowSQL, "AND") {
		rowSQL = rowSQL[:len(rowSQL)-4]
	}
	return rowSQL, nil
}
