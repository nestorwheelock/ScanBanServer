package main

import (
	"strconv"
	"strings"
)

func isIPValid(ip string) bool {
	if strings.HasPrefix(ip, ".") || strings.HasSuffix(ip, ".") || strings.Count(ip, ".") != 3 {
		return false
	}
	for _, p := range strings.Split(ip, ".") {
		part, err := strconv.Atoi(p)
		if err != nil {
			return false
		}
		if part > 255 || part < 0 {
			return false
		}
	}
	_, err := strconv.Atoi(strings.ReplaceAll(ip, ".", ""))
	return err == nil
}

func removeIP(iplist []IPset, ip string) []IPset {
	var newIPs []IPset
	for _, cip := range iplist {
		if cip.IP != ip {
			newIPs = append(newIPs, cip)
		}
	}
	return newIPs
}

func concatIPList(ips []IPset) string {
	iplist := "\""
	for _, ip := range ips {
		iplist += ip.IP + "\",\""
	}
	return iplist[:len(iplist)-2]
}

func ipidFromIP(ipids []IPID, ip string) *IPID {
	for _, ipid := range ipids {
		if ipid.IP == ip {
			return &ipid
		}
	}
	return nil
}
