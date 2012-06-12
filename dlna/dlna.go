package dlna

import (
	"fmt"
	//"math"
	"strconv"
	"strings"
	"time"
)

const (
	TimeSeekRangeDomain   = "TimeSeekRange.dlna.org"
	ContentFeaturesDomain = "contentFeatures.dlna.org"
	TransferModeDomain    = "transferMode.dlna.org"
)

type ContentFeatures struct {
	SupportTimeSeek bool
	SupportRange    bool
	Transcoded      bool
}

// flags are in hex. trailing 24 zeroes, 26 are after the space
// "DLNA.ORG_OP=" time-seek-range-supp bytes-range-header-supp

func (cf ContentFeatures) String() string {
	return fmt.Sprintf("DLNA.ORG_OP=%02b;DLNA.ORG_CI=%b;DLNA.ORG_FLAGS=017000 00000000000000000000000000000000", func() (ret uint) {
		if cf.SupportTimeSeek {
			ret |= 2
		}
		if cf.SupportRange {
			ret |= 1
		}
		return
	}(), func() uint {
		if cf.Transcoded {
			return 1
		}
		return 0
	}())
}

func ParseNPTDuration(s string) (ret time.Duration, err error) {
	ss := strings.SplitN(s, ":", 3)
	var (
		h   int64
		m   uint64
		sec float64
	)
	h, err = strconv.ParseInt(ss[0], 0, 64)
	if err != nil {
		return
	}
	m, err = strconv.ParseUint(ss[1], 0, 0)
	if err != nil {
		return
	}
	sec, err = strconv.ParseFloat(ss[2], 64)
	if err != nil {
		return
	}
	ret = time.Duration(h) * time.Hour
	ret += time.Duration(m) * time.Minute
	ret += time.Duration(sec * float64(time.Second))
	return
}

func NPTDurationAsString(d time.Duration)

type NPTRange struct {
	Start, End time.Duration
}

func ParseNPTRange(s string) (ret NPTRange, err error) {
	ss := strings.SplitN(s, "-", 2)
	if ss[0] != "" {
		ret.Start, err = ParseNPTDuration(ss[0])
		if err != nil {
			return
		}
	}
	if ss[1] != "" {
		ret.End, err = ParseNPTDuration(ss[1])
		if err != nil {
			return
		}
	}
	return
}

func (me NPTRange) String() (ret string) {
	ret = me.Start.String() + "-"
	if me.End >= 0 {
		ret += me.End.String()
	}
	return
}
