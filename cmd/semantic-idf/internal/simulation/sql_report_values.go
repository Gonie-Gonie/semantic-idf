package simulation

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// walkReportDataCompact preserves the public walker's time/dictionary ordering,
// filters and row semantics, but does not sort and decode dictionary strings
// for every observation. Only the three ReportData values cross the large-row
// SQL boundary; dictionary and Time metadata are loaded once per invocation.
// It deliberately does not create indexes or otherwise modify the database.
func walkReportDataCompact(db *sql.DB, query SQLSeriesQuery, visit func(SQLSeriesRow) error) error {
	rd, err := sqlTableColumns(db, "ReportData")
	if err != nil {
		return err
	}
	rdd, err := sqlTableColumns(db, "ReportDataDictionary")
	if err != nil {
		return err
	}
	times, err := sqlTableColumns(db, "Time")
	if err != nil {
		return err
	}
	column := func(columns map[string]string, name string) string {
		return columns[normalizeSQLColumnName(name)]
	}
	rdTime, rdDictionary, rdValue := column(rd, "TimeIndex"), column(rd, "ReportDataDictionaryIndex"), column(rd, "Value")
	dictionaryID, timeID := column(rdd, "ReportDataDictionaryIndex"), column(times, "TimeIndex")
	if rdTime == "" || rdDictionary == "" || rdValue == "" || dictionaryID == "" || timeID == "" {
		return walkReportData(db, query, visit)
	}
	for table, keys := range map[string][]string{
		"ReportData": {rdTime, rdDictionary}, "ReportDataDictionary": {dictionaryID}, "Time": {timeID},
	} {
		if !compactReportDataIntegerKeyAffinity(db, table, keys) {
			// TEXT '01' and INTEGER 1 have different SQLite equality semantics.
			// Do not replace a nonstandard SQL join with integer map lookups.
			return walkReportData(db, query, visit)
		}
	}
	idExpr := "rdd." + quoteSQLiteIdentifier(dictionaryID)
	keyExpr := sqlAliasedTextColumnExpr(rdd, "rdd", "KeyValue", "''")
	nameExpr := sqlAliasedTextColumnExpr(rdd, "rdd", "Name", "''")
	unitExpr := sqlAliasedTextColumnExpr(rdd, "rdd", "Units", "''")
	meterExpr := sqlAliasedCastTextColumnExpr(rdd, "rdd", "IsMeter", "'0'")
	frequencyExpr := sqlAliasedTextColumnExpr(rdd, "rdd", "ReportingFrequency", "''")
	groupExpr := sqlAliasedTextColumnExpr(rdd, "rdd", "IndexGroup", "''")
	where, args := sqlSeriesQueryWhereClause(query, idExpr, nameExpr, keyExpr, meterExpr, frequencyExpr, unitExpr, groupExpr)
	rows, err := db.Query(fmt.Sprintf(`SELECT %s, %s, %s, %s, %s, %s, %s, typeof(%s)
FROM ReportDataDictionary rdd %s`, idExpr, keyExpr, nameExpr, unitExpr, meterExpr, frequencyExpr, groupExpr, idExpr, where), args...)
	if err != nil {
		return err
	}
	dictionaries := map[int]SQLSeriesRow{}
	ids := []string{}
	for rows.Next() {
		var id sql.NullInt64
		var item SQLSeriesRow
		var meter, storage string
		if err := rows.Scan(&id, &item.KeyValue, &item.Name, &item.Units, &meter, &item.ReportingFrequency, &item.IndexGroup, &storage); err != nil {
			rows.Close()
			return walkReportData(db, query, visit)
		}
		if !id.Valid {
			continue // SQL NULL cannot participate in the original inner join.
		}
		if storage != "integer" {
			rows.Close()
			return walkReportData(db, query, visit)
		}
		item.DictionaryIndex, item.IsMeter = int(id.Int64), parseSQLBool(meter)
		if _, duplicate := dictionaries[item.DictionaryIndex]; duplicate {
			rows.Close()
			// Nonstandard schemas can make a join one-to-many. Do not collapse it.
			return walkReportData(db, query, visit)
		}
		dictionaries[item.DictionaryIndex] = item
		ids = append(ids, strconv.FormatInt(id.Int64, 10))
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(dictionaries) == 0 {
		return nil
	}
	rows, err = db.Query(fmt.Sprintf(`SELECT t.%s, %s, %s, %s, %s, %s, typeof(t.%s) FROM "Time" t`,
		quoteSQLiteIdentifier(timeID),
		sqlAliasedColumnExpr(times, "t", "Month", "NULL"),
		sqlAliasedColumnExpr(times, "t", "Day", "NULL"),
		sqlAliasedColumnExpr(times, "t", "Hour", "NULL"),
		sqlAliasedColumnExpr(times, "t", "Minute", "NULL"),
		sqlAliasedTextColumnExpr(times, "t", "IntervalType", "''"), quoteSQLiteIdentifier(timeID)))
	if err != nil {
		return err
	}
	frames := map[int64]SQLSeriesRow{}
	for rows.Next() {
		var id sql.NullInt64
		var frame SQLSeriesRow
		var storage string
		if err := rows.Scan(&id, &frame.Month, &frame.Day, &frame.Hour, &frame.Minute, &frame.IntervalType, &storage); err != nil {
			rows.Close()
			return walkReportData(db, query, visit)
		}
		if !id.Valid {
			continue
		}
		if storage != "integer" {
			rows.Close()
			return walkReportData(db, query, visit)
		}
		if _, duplicate := frames[id.Int64]; duplicate {
			rows.Close()
			return walkReportData(db, query, visit)
		}
		frames[id.Int64] = frame
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	rows, err = db.Query(fmt.Sprintf(`SELECT %s, %s, %s FROM ReportData
WHERE %s IN (%s) ORDER BY %s, %s`, quoteSQLiteIdentifier(rdTime), quoteSQLiteIdentifier(rdDictionary), quoteSQLiteIdentifier(rdValue),
		quoteSQLiteIdentifier(rdDictionary), strings.Join(ids, ","), quoteSQLiteIdentifier(rdTime), quoteSQLiteIdentifier(rdDictionary)))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var rawTimeIndex any
		var dictionaryIndex int
		var value sql.NullFloat64
		if err := rows.Scan(&rawTimeIndex, &dictionaryIndex, &value); err != nil {
			return err
		}
		var timeIndex sql.NullInt64
		if err := timeIndex.Scan(rawTimeIndex); err != nil {
			return err
		}
		if !timeIndex.Valid {
			return fmt.Errorf("ReportData TimeIndex is NULL")
		}
		row := dictionaries[dictionaryIndex]
		var frame SQLSeriesRow
		if _, integer := rawTimeIndex.(int64); integer {
			frame = frames[timeIndex.Int64]
		}
		// A BLOB such as x'3031' scans as integer 1, but cannot SQL-join an
		// INTEGER Time ID. Preserve its observation with LEFT JOIN NULL metadata.
		row.TimeIndex, row.Value = timeIndex.Int64, value
		row.Month, row.Day, row.Hour, row.Minute, row.IntervalType = frame.Month, frame.Day, frame.Hour, frame.Minute, frame.IntervalType
		if err := visit(row); err != nil {
			return err
		}
	}
	return rows.Err()
}

func compactReportDataIntegerKeyAffinity(db *sql.DB, table string, keys []string) bool {
	rows, err := db.Query("PRAGMA table_info(" + quoteSQLiteIdentifier(table) + ")")
	if err != nil {
		return false
	}
	defer rows.Close()
	wanted := map[string]bool{}
	for _, key := range keys {
		wanted[normalizeSQLColumnName(key)] = true
	}
	for rows.Next() {
		var position, notNull, primaryKey int
		var name, declaredType string
		var defaultValue any
		if err := rows.Scan(&position, &name, &declaredType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false
		}
		key := normalizeSQLColumnName(name)
		if wanted[key] {
			if !strings.Contains(strings.ToUpper(declaredType), "INT") {
				return false
			}
			delete(wanted, key)
		}
	}
	return rows.Err() == nil && len(wanted) == 0
}
