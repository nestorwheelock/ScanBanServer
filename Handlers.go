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

func reportIPs2(w http.ResponseWriter, r *http.Request) {
	var report ReportStruct

	if !handleUserInput(w, r, &report) {
		return
	}
	if isStructInvalid(report) {
		sendError("input missing", w, WrongInputFormatError, 422)
		return
	}

	ipheader := []string{"X-Forwarded-For", "X-Real-Ip", "HTTP_CLIENT_IP", "HTTP_X_FORWARDED_FOR", "HTTP_X_FORWARDED", "HTTP_X_CLUSTER_CLIENT_IP", "HTTP_FORWARDED_FOR", "HTTP_FORWARDED", "REMOTE_ADDR"}
	var repIP string
	for _, header := range ipheader {
		cip := r.Header.Get(header)
		cip = strings.Trim(cip, " ")
		if len(cip) > 0 {
			repIP = cip
			break
		}
	}
	remAddr := false
	if len(strings.Trim(repIP, " ")) == 0 {
		repIP = r.RemoteAddr
		remAddr = true
	}
	if strings.Contains(repIP, ":") {
		repIP = repIP[:(strings.LastIndex(repIP, ":"))]
	}

	if remAddr {
		LogInfo("Reporter: \"" + repIP + "\" (rem addr)")
	} else {
		LogInfo("Reporter: \"" + repIP + "\"")
	}

	ips := []IPData{}
	validIPfound := false
	ownIP := getOwnIP()
	for _, ip := range report.IPs {
		sIP := ip.IP
		if valid, reas := isIPValid(sIP); valid && sIP != repIP && sIP != ownIP {
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
		ret := insertIPs2(report.Token, ips, report.StartTime)
		handleError(sendSuccess(w, ret), w, ServerError, 500)
	} else {
		sendError("no valid ip found in report", w, NoValidIPFound, 422)
		return
	}
}

func reportIPs(w http.ResponseWriter, r *http.Request) {
	var report ReportIPStruct

	if !handleUserInput(w, r, &report) {
		return
	}

	if isStructInvalid(report) {
		sendError("input missing", w, WrongInputFormatError, 422)
		return
	}

	if len(report.Token) != 64 {
		sendError("wrong token length", w, InvalidTokenError, 422)
		return
	}

	if len(report.Ips) == 0 {
		sendError("No ip given", w, WrongInputFormatError, 422)
		return
	}

	ipheader := []string{"X-Forwarded-For", "X-Real-Ip", "HTTP_CLIENT_IP", "HTTP_X_FORWARDED_FOR", "HTTP_X_FORWARDED", "HTTP_X_CLUSTER_CLIENT_IP", "HTTP_FORWARDED_FOR", "HTTP_FORWARDED", "REMOTE_ADDR"}
	var repIP string
	for _, header := range ipheader {
		cip := r.Header.Get(header)
		cip = strings.Trim(cip, " ")
		if len(cip) > 0 {
			repIP = cip
			break
		}
	}
	if len(strings.Trim(repIP, " ")) == 0 {
		LogInfo("Using rem addr")
		repIP = r.RemoteAddr
	}
	if strings.Contains(repIP, ":") {
		repIP = repIP[:(strings.LastIndex(repIP, ":"))]
	}

	validFound := false
	var validIPs []IPset
	for _, i := range report.Ips {
		valid, _ := isIPValid(i.IP)
		if !isStructInvalid(i) && valid && (i.Reason > 0 && i.Reason < 4) && i.IP != repIP {
			validFound = true
			validIPs = append(validIPs, i)
		}
	}

	if len(validIPs) == 0 {
		sendError("No valid ip found", w, WrongInputFormatError, 404)
		return
	}

	if !validFound {
		sendError("input missing", w, WrongInputFormatError, 422)
		return
	}
	note := ""
	if report.Note != nil && len(*report.Note) > 0 {
		note = *report.Note
	}
	returnCode := insertIPs(report.Token, note, validIPs)
	resp := ""
	if returnCode == 1 {
		resp = "ok"
	} else if returnCode == -1 {
		resp = "user invalid"
	} else if returnCode == -3 {
		resp = "too many ips!"
	} else {
		resp = "server error"
	}
	handleError(sendSuccess(w, resp), w, ServerError, 500)

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

	ips, err := fetchIPsFromDB(fetchRequest.Token, fetchRequest.Filter)
	if err == -1 {
		sendError("User invalid", w, InvalidTokenError, 422)
		return
	} else if err >= 0 {
		if len(ips) == 0 {
			handleError(sendSuccess(w, "[]"), w, ServerError, 500)
		} else {
			fetchresponse := FetchResponse{
				IPs:              ips,
				CurrentTimestamp: time.Now().Unix(),
			}
			handleError(sendSuccess(w, fetchresponse), w, ServerError, 500)
		}
	} else {
		sendError("Server error", w, ServerError, 422)
	}
}

func handleUserInput(w http.ResponseWriter, r *http.Request, p interface{}) bool {
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 10000))
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
