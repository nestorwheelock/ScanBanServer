package main

import (
	"errors"
	"strconv"
)

//ReportIPcount ip and count
type ReportIPcount struct {
	Count uint `db:"c"`
	ID    uint `db:"iid"`
}

//IPDataResult result from inserted IP in database
type IPDataResult struct {
	IP   string
	IPID uint
}

const batchSize = 30

//---------------------------------------- INSERT report ---------------------------------------

func insertIPs(token string, ipdatas []IPData, starttime uint64) int {
	valid, uid, permissions := IsUserValid(token)

	if !valid {
		return -1
	}

	if !canUser(permissions, PushIPs) {
		return -3
	}

	go (func() {
		execDB("UPDATE Token SET reportedIPs=reportedIPs+?, lastReport=now() WHERE pk_id=?", len(ipdatas), uid)
	})()

	ipdataresult := []IPDataResult{}
	var cdone chan bool
	for _, ipdata := range ipdatas {
		cdone = make(chan bool, 1)
		ipID, reportID, err := insertIP(ipdata, uid)
		if ipID == 0 {
			ipID = getIPID(ipdata.IP)
			if ipID == 0 {
				return -2
			}
		}

		if err != nil {
			LogCritical("Error inserting ip: " + err.Error())
			continue
		}
		if reportID == 0 {
			reportID, err = insertReport(ipID, uid)
			if err != nil {
				LogCritical("Error inserting report: " + err.Error())
				continue
			}
		}
		go (func() {
			execDB("UPDATE Report SET lastReport=(SELECT UNIX_TIMESTAMP()) WHERE pk_id=?", reportID)
		})()

		for _, iPPort := range ipdata.Ports {
			if len(iPPort.Times) == 0 || iPPort.Port < 1 || iPPort.Port > 65535 {
				LogInfo("IP data invalid: " + ipdata.IP + ":" + strconv.Itoa(iPPort.Port))
				continue
			}
			batches := make(map[int][]int)
			for _, time := range iPPort.Times {
				pos := (int)(time / batchSize)
				_, ok := batches[pos]
				if !ok {
					batches[pos] = []int{time}
				} else {
					batches[pos] = append(batches[pos], time)
				}
			}
			insertBatch(batches, reportID, iPPort, starttime, ipID, cdone)
		}

		ipdataresult = append(ipdataresult, IPDataResult{
			IP:   ipdata.IP,
			IPID: ipID,
		})
	}

	go (func() {
		<-cdone
		for _, ipdata := range ipdataresult {
			filterbuilder.addIP(ipdata)
		}
	})()

	return 1
}

func insertBatch(batch map[int][]int, reportID uint, ipportreport IPPortReport, startTime uint64, ipID uint, cdone chan bool) {
	values := ""
	for _, b := range batch {
		scanCount := len(b)
		if scanCount == 0 {
			continue
		}
		var rpID int
		err := queryRow(&rpID, "SELECT IFNULL(MAX(pk_id),-1) FROM ReportPorts WHERE scanDate >= ? AND reportID=? AND port=?", startTime-uint64(batchSize), reportID, ipportreport.Port)
		if err != nil {
			LogCritical("Couldn't get reportPorts in current batch: " + err.Error())
			continue
		}

		if rpID > 0 {
			go (func() {
				execDB("UPDATE ReportPorts SET count=count+? WHERE pk_id=?", scanCount, rpID)
			})()
		} else {
			values += "(" + strconv.FormatUint(uint64(reportID), 10) + "," + strconv.Itoa(ipportreport.Port) + "," + strconv.Itoa(scanCount) + "," + strconv.FormatUint(startTime, 10) + "),"
		}

		var c int
		err = queryRow(&c, "SELECT COUNT(ip) FROM IPports WHERE ip=? AND port=?", ipID, ipportreport.Port)
		if err != nil {
			LogCritical("Error retrieving ipport: " + err.Error())
		} else {
			go (func() {
				if c > 0 {
					execDB("UPDATE IPports SET count=count+? WHERE ip=? AND port=?", scanCount, ipID, ipportreport.Port)
				} else {
					execDB("INSERT INTO IPports (ip, port, count) VALUES(?,?,?)", ipID, ipportreport.Port, scanCount)
				}
				cdone <- true
			})()
		}
	}

	if len(values) > 2 {
		go (func() {
			err := execDB("INSERT INTO ReportPorts (reportID, port, count, scanDate) VALUES" + values[:len(values)-1])
			if err != nil {
				LogCritical("Couldn't insert ReportPort: " + err.Error())
			}
		})()
	}
}

func getIPID(ip string) uint {
	var ipid uint
	err := queryRow(&ipid, "SELECT BlockedIP.pk_id FROM BlockedIP WHERE BlockedIP.ip=?", ip)
	if err != nil {
		LogCritical("Error getting IP")
		return 0
	}
	return ipid
}

func insertReport(ip uint, uid uint) (uint, error) {
	r, err := db.Exec("INSERT INTO Report (ip, reporterID, firstReport) VALUES(?,?,(SELECT UNIX_TIMESTAMP()))", ip, uid)
	if err != nil {
		return 0, err
	}
	id, err := r.LastInsertId()
	if err != nil || id < 1 {
		LogInfo("Server does not support getting last inserted ID:" + err.Error())
		var id uint
		err = queryRow(&id, "SELECT Report.pk_id FROM Report WHERE ip=? AND reporterID=?", ip, uid)
		if err != nil {
			return 0, err
		} else if id == 0 {
			return 0, errors.New("report not found")
		}
		return id, nil
	}
	return uint(id), nil
}

func insertIP(ipdata IPData, uid uint) (IPid uint, reportID uint, err error) {
	IPid, reportID = 0, 0
	err = nil

	var c ReportIPcount
	err = queryRow(&c, "SELECT COUNT(*) as c, ifnull(pk_id, 0)as iid FROM Report WHERE reporterID=? AND ip=ifnull((SELECT BlockedIP.pk_id FROM BlockedIP WHERE BlockedIP.ip=?),\"\")", uid, ipdata.IP)
	if err != nil {
		return
	}
	if c.Count != 0 {
		reportID = c.ID
		return
	}
	var ce int
	err = queryRow(&ce, "SELECT COUNT(*) FROM BlockedIP WHERE ip=?", ipdata.IP)
	if err != nil {
		return
	}
	err = execDB("INSERT INTO BlockedIP (ip, validated,firstReport, lastReport) VALUES (?,0,(SELECT UNIX_TIMESTAMP()),(SELECT UNIX_TIMESTAMP())) ON DUPLICATE KEY UPDATE reportCount=reportCount+1, deleted=0, lastReport=(SELECT UNIX_TIMESTAMP())", ipdata.IP)
	if err != nil {
		return
	}
	if ce == 0 {
		doAnalytics(ipdata)
	}
	err = queryRow(&IPid, "SELECT BlockedIP.pk_id FROM BlockedIP WHERE BlockedIP.ip=?", ipdata.IP)
	if err != nil {
		return
	}
	reportID = c.ID
	return
}

//---------------------------------------- GET ipinfo ---------------------------------------

func getIPInfo(ips []string, token string) (int, *[]IPInfoData) {
	valid, _, permissions := IsUserValid(token)
	if !valid {
		return -1, nil
	}

	if !canUser(permissions, ViewReports) {
		return -3, nil
	}

	ipdata := []IPInfoData{}
	for _, ip := range ips {
		var info []ReportData
		err := queryRows(&info, "SELECT Report.reporterID, Token.machineName, ReportPorts.scanDate, ReportPorts.port, ReportPorts.count FROM `Report`"+
			"JOIN BlockedIP on (BlockedIP.pk_id = Report.ip)"+
			"JOIN Token on (Token.pk_id = Report.reporterID)"+
			"JOIN ReportPorts on (ReportPorts.reportID = Report.pk_id)"+
			"WHERE BlockedIP.ip=? ORDER BY ReportPorts.scanDate ASC", ip)
		if err != nil {
			LogCritical("Error getting info: " + err.Error())
			return 2, nil
		}
		ipdata = append(ipdata, IPInfoData{
			IP:      ip,
			Reports: info,
		})
	}

	go (func() {
		err := execDB("UPDATE Token SET requests=requests+1 WHERE token=?", token)
		if err != nil {
			LogError("Error updating requests count")
		}
	})()

	return 1, &ipdata
}

//---------------------------------------- Fetch IPs ---------------------------------------
type tokenFetchData struct {
	FilterID  uint `db:"fid"`
	FullFetch bool `db:"fullFetch"`
}

func fetchIPsFromDB(token string, filter FetchFilter) ([]IPList, bool, int) {
	valid, _, permissions := IsUserValid(token)
	if !valid {
		return nil, false, -1
	}

	if !canUser(permissions, FetchIPs) {
		return nil, false, -3
	}

	var tokenData tokenFetchData
	err := queryRow(&tokenData, "SELECT IFNULL(filter, 0)as fid,fullFetch FROM Token WHERE token=?", token)
	if err != nil || tokenData.FilterID < 0 {
		return nil, false, -2
	}

	if tokenData.FullFetch {
		filter.Since = 0
	}

	go (func() {
		execDB("UPDATE Token SET lastRequest=now() WHERE token=?", token)
	})()

	var iplist []IPList
	var query string
	if tokenData.FilterID == 0 {
		query =
			"SELECT ip,deleted AS del " +
				"FROM BlockedIP " +
				"WHERE " +
				"(lastReport >= ? OR firstReport >= ?) "
		err = queryRows(&iplist, query, filter.Since, filter.Since)
	} else {
		query = "SELECT BlockedIP.ip AS ip,0 AS del FROM BlockedIP " +
			"JOIN FilterIP ON FilterIP.ip = BlockedIP.pk_id WHERE FilterIP.filterID=? AND FilterIP.added > ? AND deleted=0 " +
			"UNION " +
			"SELECT BlockedIP.ip,1 AS del FROM FilterDelete JOIN BlockedIP ON BlockedIP.pk_id = FilterDelete.ip WHERE FilterDelete.tokenID=(SELECT pk_id FROM Token WHERE Token.token=?)"
		err = queryRows(&iplist, query, tokenData.FilterID, filter.Since, token)
		go (func() {
			execDB("DELETE FROM FilterDelete WHERE tokenID=(SELECT pk_id FROM Token WHERE Token.token=?)", token)
		})()
	}

	if err != nil {
		LogCritical("Executing fetch: " + err.Error())
		return nil, false, 1
	}

	go (func() {
		err := execDB("UPDATE Token SET requests=requests+1,fullFetch=0 WHERE token=?", token)
		if err != nil {
			LogError("Error updating requests count")
		}
	})()

	return iplist, tokenData.FullFetch, 0
}

//IsUserValid returns userid if valid or -1 if invalid
func IsUserValid(token string) (bool, uint, int16) {
	sqlCheckUserValid := "SELECT Token.pk_id, Token.permissions FROM Token WHERE token=? AND Token.isValid=1"
	var uid UserPermissions
	err := queryRow(&uid, sqlCheckUserValid, token)
	if err != nil {
		return false, 0, 0
	}
	return true, uid.UID, uid.Permissions
}

func isConnectedToDB() error {
	sqlCheckConnection := "SELECT COUNT(*) FROM Token"
	var count int
	err := queryRow(&count, sqlCheckConnection)
	if err != nil {
		return err
	}
	return nil
}

func min(intsl []int) int {
	if len(intsl) == 0 {
		return 0
	}
	if len(intsl) == 1 {
		return intsl[0]
	}
	ix := intsl[0]
	for _, i := range intsl {
		if i < ix {
			ix = i
		}
	}
	return ix
}

func max(intsl []int) int {
	if len(intsl) == 0 {
		return 0
	}
	if len(intsl) == 1 {
		return intsl[0]
	}
	ix := intsl[0]
	for _, i := range intsl {
		if i > ix {
			ix = i
		}
	}
	return ix
}
