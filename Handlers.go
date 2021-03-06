package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"
	"time"
)

func ping(w http.ResponseWriter, r *http.Request) {
	var pingRequest PingRequest

	if !handleUserInput(w, r, &pingRequest) {
		return
	}
	if isStructInvalid(pingRequest) {
		sendError("input missing", w, WrongInputFormatError, 422)
		return
	}
	if len(pingRequest.Token) != 64 {
		sendError("wrong token length", w, InvalidTokenError, 422)
		return
	}
	isValid, _, _ := IsUserValid(pingRequest.Token)
	if isValid {
		handleError(sendSuccess(w, Status{
			StatusCode:    "success",
			StatusMessage: "success",
		}), w, ServerError, 500)
	} else {
		sendError("user invalid", w, InvalidTokenError, 403)
	}
}

func fetchIPInfo(w http.ResponseWriter, r *http.Request) {
	var ipinforequest IPInfoRequest
	if !handleUserInput(w, r, &ipinforequest) {
		return
	}
	if isStructInvalid(ipinforequest) {
		sendError("input missing", w, WrongInputFormatError, 422)
		return
	}
	if len(ipinforequest.Token) != 64 {
		sendError("wrong token length", w, InvalidTokenError, 422)
		return
	}

	ips := []string{}
	validIPfound := false
	ownIP := getOwnIP()
	for _, ip := range ipinforequest.IPs {
		if valid, reas := isIPValid(ip); valid && ip != ownIP {
			ips = append(ips, ip)
			validIPfound = true
		} else {
			add := ""
			if reas != 1 {
				if reas == 0 {
					add = "No valid ipv4"
				} else if reas == -1 {
					add = "IP is reserved"
				}
			} else if ip == ownIP {
				add = "IP is servers IP"
			}
			LogInfo("IP \"" + ip + "\" is not valid! " + add)
		}
	}

	if validIPfound {
		c, data := getIPInfo(ips, ipinforequest.Token)
		if c == -1 {
			sendError("User invalid", w, InvalidTokenError, 422)
			return
		} else if c == 2 {
			sendError("Server error", w, ServerError, 422)
			return
		} else if c == -3 {
			sendError("No permission", w, NoPermissionError, 403)
			return
		}
		handleError(sendSuccess(w, data), w, ServerError, 500)
	} else {
		sendError("no valid ip found in report", w, NoValidIPFound, 422)
		return
	}
}

func reportIPs(w http.ResponseWriter, r *http.Request) {
	var report ReportStruct

	if !handleUserInput(w, r, &report) {
		return
	}
	if isStructInvalid(report) {
		sendError("input missing", w, WrongInputFormatError, 422)
		return
	}

	repIP := getIPFromHTTPrequest(r)
	var whitelisted []string
	queryRows(&whitelisted, "SELECT ip FROM IPwhitelist")

	ips := []IPData{}
	validIPfound := false
	ownIP := getOwnIP()
	for _, ip := range report.IPs {
		sIP := ip.IP
		if isInStringArray(sIP, whitelisted, true) {
			LogInfo("IP " + sIP + " is in whitelist")
		} else if valid, reas := isIPValid(sIP); valid && sIP != repIP && sIP != ownIP {
			ips = append(ips, ip)
			validIPfound = true
		} else {
			add := ""
			if sIP == repIP {
				add = "IP is reporters IP"
			} else if reas != 1 {
				if reas == 0 {
					add = "No valid ipv4"
				} else if reas == -1 {
					add = "IP is reserved"
				}
			} else if sIP == ownIP {
				add = "IP is servers IP"
			}
			LogInfo("IP \"" + sIP + "\" is not valid! " + add)
		}
	}

	if validIPfound {
		c := insertIPs(report.Token, ips, report.StartTime)
		if c == -1 {
			sendError("User invalid", w, InvalidTokenError, 422)
			return
		} else if c == 2 {
			sendError("Server error", w, ServerError, 422)
			return
		} else if c == -3 {
			sendError("No permission", w, NoPermissionError, 403)
			return
		}
		handleError(sendSuccess(w, Status{
			StatusCode:    "success",
			StatusMessage: "success",
		}), w, ServerError, 500)
	} else {
		sendError("no valid ip found in report", w, NoValidIPFound, 422)
		return
	}
}

func fetchIPs(w http.ResponseWriter, r *http.Request) {
	var fetchRequest FetchRequest

	if !handleUserInput(w, r, &fetchRequest) {
		return
	}

	if isStructInvalid(fetchRequest) {
		sendError("input missing", w, WrongInputFormatError, 422)
		return
	}

	if len(fetchRequest.Token) != 64 {
		sendError("wrong token length", w, InvalidTokenError, 422)
		return
	}

	start := time.Now()
	ips, fullFetch, err := fetchIPsFromDB(fetchRequest.Token, fetchRequest.Filter)
	fmt.Println(time.Now().Sub(start).String())
	if err == -1 {
		sendError("User invalid", w, InvalidTokenError, 422)
		return
	} else if err == -2 {
		sendError("Server error", w, ServerError, 422)
		return
	} else if err == -3 {
		sendError("No permission", w, NoPermissionError, 403)
		return
	} else if err >= 0 {
		if len(ips) == 0 {
			handleError(sendSuccess(w, []string{}), w, ServerError, 500)
		} else {
			fetchresponse := FetchResponse{
				IPs:              ips,
				CurrentTimestamp: time.Now().Unix(),
				Full:             fullFetch,
			}
			handleError(sendSuccess(w, fetchresponse), w, ServerError, 500)
		}
	} else {
		sendError("Server error", w, ServerError, 422)
	}
}

func initNewToken(w http.ResponseWriter, r *http.Request) {
	repIP := getIPFromHTTPrequest(r)
	if repIP == "127.0.0.1" || repIP == "[::1]" {
		var ifr InitNewTokenRequest

		if !handleUserInput(w, r, &ifr) {
			return
		}

		if isStructInvalid(ifr) {
			sendError("input missing", w, WrongInputFormatError, 422)
			return
		}

		if ifr.AuthToken == "83fab411fb34c09bb7f6563a3e36fdc67d40c81d8a77936e48df6f6ad3ff4e7c46fca610e3253211e2708910829f6842db02345e64562a86fa7c2618ede5c286" {
			needNew := false
			fmt.Println("filterID", ifr.FilterID)
			fmt.Println("ifr.mode=", ifr.Mode)
			if ifr.Mode == 2 || ifr.Mode == 3 {
				needNew = true
				err := execDB("INSERT INTO Token (machineName, token, permissions, filter, fullFetch) VALUES(?,?,?,?,1)", ifr.Name, ifr.Token, ifr.Permission, ifr.FilterID)
				if err != nil {
					sendError("server error: "+err.Error(), w, ServerError, 500)
					return
				}
				if ifr.Mode == 2 {
					var c int
					err = queryRow(&c, "SELECT COUNT(filter) FROM Token WHERE filter=?", ifr.FilterID)
					if err != nil {
						sendError("server error: "+err.Error(), w, ServerError, 500)
						return
					}
					fmt.Println("c:", c)
					if c == 1 {
						needNew = true
					} else {
						needNew = false
					}
				}
				if needNew {
					err = execDB("INSERT INTO FilterChange (del, filterID) VALUES(0,?)", ifr.FilterID)
					if err != nil {
						sendError("server error: "+err.Error(), w, ServerError, 500)
						return
					}
				}
			} else if ifr.Mode == 1 {
				err := execDB("INSERT INTO Token (machineName, token, permissions, filter, fullFetch) VALUES(?,?,?,NULL,1)", ifr.Name, ifr.Token, ifr.Permission)
				if err != nil {
					sendError("server error: "+err.Error(), w, ServerError, 500)
					return
				}
			}
			err := execDB("INSERT INTO UserMachines (userID, token) VALUES(?,(SELECT MAX(pk_id) FROM Token WHERE token=?))", ifr.UID, ifr.Token)
			if err != nil {
				sendError("server error: "+err.Error(), w, ServerError, 500)
				return
			}
			if needNew {
				filterID := ifr.FilterID
				filterbuilder.updateCachedFilter(false)
				for i, filter := range filterbuilder.filter {
					fmt.Println("looping filter:", filter.ID)
					if filter.ID == filterID {
						filterbuilder.filter[i].Skip = false
						wheresql, joinAdd, err := getFilterSQL(filter, "BlockedIP.pk_id")
						if err != nil {
							LogCritical("Error getting filterWhere: " + err.Error())
							return
						}
						sql := "INSERT INTO FilterIP (ip, filterID, added) " +
							"(SELECT BlockedIP.pk_id, ?, ? FROM BlockedIP " + joinAdd + " WHERE " +
							wheresql +
							")"
						fmt.Println(sql)
						err = execDB(sql, filterID, time.Now().Unix())
						if err != nil {
							LogInfo(sql)
							LogCritical("Error filtering IPs: " + err.Error())
							return
						}
						break
					}
				}
			}
			handleError(sendSuccess(w, Status{
				StatusCode:    "success",
				StatusMessage: "success",
			}), w, ServerError, 500)
		} else {
			LogError("Invalid authToken for ifr")
		}
	} else {
		LogInfo("Request from not localhost")
	}
}
func updateFilterCache(w http.ResponseWriter, r *http.Request) {
	repIP := getIPFromHTTPrequest(r)
	if repIP == "127.0.0.1" || repIP == "[::1]" {
		var ufr UpdateFilterCacheRequest

		if !handleUserInput(w, r, &ufr) {
			return
		}

		if isStructInvalid(ufr) {
			sendError("input missing", w, WrongInputFormatError, 422)
			return
		}

		if ufr.AuthToken == "83fab411fb34c09bb7f6563a3e36fdc67d40c81d8a77936e48df6f6ad3ff4e7c46fca610e3253211e2708910829f6842db02345e64562a86fa7c2618ede5c286" {
			filterbuilder.updateCachedFilter(false)
		}
	}
}

func handleUserInput(w http.ResponseWriter, r *http.Request, p interface{}) bool {
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 40000000))
	if err != nil {
		LogError("ReadError: " + err.Error())
		return false
	}
	if err := r.Body.Close(); err != nil {
		LogError("ReadError: " + err.Error())
		return false
	}

	errEncode := json.Unmarshal(body, p)
	if handleError(errEncode, w, WrongInputFormatError, 422) {
		return false
	}
	return true
}

func handleError(err error, w http.ResponseWriter, message ErrorMessage, statusCode int) bool {
	if err == nil {
		return false
	}
	sendError(err.Error(), w, message, statusCode)
	return true
}

func sendError(erre string, w http.ResponseWriter, message ErrorMessage, statusCode int) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	if statusCode >= 500 {
		LogCritical(erre)
	} else {
		LogError(erre)
	}
	w.WriteHeader(statusCode)

	var de []byte
	var err error
	if len(string(message)) == 0 {
		de, err = json.Marshal(&ResponseError)
	} else {
		de, err = json.Marshal(&Status{"error", string(message)})
	}

	if err != nil {
		panic(err)
	}
	_, _ = fmt.Fprintln(w, string(de))
}

func isStructInvalid(x interface{}) bool {
	s := reflect.TypeOf(x)
	for i := s.NumField() - 1; i >= 0; i-- {
		e := reflect.ValueOf(x).Field(i)

		if isEmptyValue(e) {
			return true
		}
	}
	return false
}

func isEmptyValue(e reflect.Value) bool {
	switch e.Type().Kind() {
	case reflect.String:
		if e.String() == "" || strings.Trim(e.String(), " ") == "" {
			return true
		}
	case reflect.Int:
		{
			return false
		}
	case reflect.Int64:
		{
			return false
		}
	case reflect.Array:
		for j := e.Len() - 1; j >= 0; j-- {
			isEmpty := isEmptyValue(e.Index(j))
			if isEmpty {
				return true
			}
		}
	case reflect.Slice:
		return isStructInvalid(e)
	case reflect.Uintptr:
		{
			return false
		}
	case reflect.Ptr:
		{
			return false
		}
	case reflect.UnsafePointer:
		{
			return false
		}
	case reflect.Struct:
		{
			return false
		}
	case reflect.Uint64:
		{
			return false
		}
	case reflect.Uint:
		{
			return false
		}
	case reflect.Uint8:
		{
			return false
		}
	default:
		fmt.Println(e.Type().Kind(), e)
		return true
	}
	return false
}

func sendSuccess(w http.ResponseWriter, i interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	de, err := json.Marshal(i)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(de))
	if err != nil {
		return err
	}
	return nil
}
