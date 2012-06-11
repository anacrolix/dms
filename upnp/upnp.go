package upnp

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var serviceURNRegexp *regexp.Regexp

func init() {
	var err error
	serviceURNRegexp, err = regexp.Compile(`^urn:schemas-upnp-org:service:(\w+):(\d+)$`)
	if err != nil {
		panic(err)
	}
}

type ServiceURN struct {
	Type    string
	Version uint64
}

func (me ServiceURN) String() string {
	return fmt.Sprintf("urn:schemas-upnp-org:service:%s:%d", me.Type, me.Version)
}

func ParseServiceType(s string) (ret ServiceURN, ok bool) {
	matches := serviceURNRegexp.FindStringSubmatch(s)
	if matches == nil {
		return
	}
	if len(matches) != 3 {
		panic(matches)
	}
	ret.Type = matches[1]
	var err error
	ret.Version, err = strconv.ParseUint(matches[2], 0, 0)
	if err != nil {
		return
	}
	return
}

type SoapAction struct {
	ServiceURN
	Action string
}

func ParseActionHTTPHeader(s string) (ret SoapAction, ok bool) {
	if s[0] != '"' || s[len(s)-1] != '"' {
		return
	}
	s = s[1 : len(s)-1]
	hashIndex := strings.LastIndex(s, "#")
	if hashIndex == -1 {
		return
	}
	ret.Action = s[hashIndex+1:]
	ret.ServiceURN, ok = ParseServiceType(s[:hashIndex])
	ok = true
	return
}
