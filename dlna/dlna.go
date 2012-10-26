package dlna

import (
	"fmt"
	"strings"
	"time"
)

const (
	TimeSeekRangeDomain   = "TimeSeekRange.dlna.org"
	ContentFeaturesDomain = "contentFeatures.dlna.org"
	TransferModeDomain    = "transferMode.dlna.org"
)

type ContentFeatures struct {
	ProfileName     string
	SupportTimeSeek bool
	SupportRange    bool
	//play speeds, DLNA.ORG_PS
	Transcoded bool
}

// flags are in hex. trailing 24 zeroes, 26 are after the space
// "DLNA.ORG_OP=" time-seek-range-supp bytes-range-header-supp

func (cf ContentFeatures) String() (ret string) {
	//DLNA.ORG_PN=[a-zA-Z0-9_]*
	ret = fmt.Sprintf("DLNA.ORG_OP=%02b;DLNA.ORG_CI=%b", func() (ret uint) {
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
	if cf.ProfileName != "" {
		ret += ";DLNA_PN=" + cf.ProfileName
	}
	return
}

func ParseNPTTime(s string) (time.Duration, error) {
	var h, m, sec, ms time.Duration
	n, err := fmt.Sscanf(s, "%d:%2d:%2d.%3d", &h, &m, &sec, &ms)
	if err != nil {
		return -1, err
	}
	if n < 3 {
		return -1, fmt.Errorf("invalid npt time: %s", s)
	}
	ret := time.Duration(h) * time.Hour
	ret += time.Duration(m) * time.Minute
	ret += sec * time.Second
	ret += ms * time.Millisecond
	return ret, nil
}

func FormatNPTTime(npt time.Duration) string {
	npt /= time.Millisecond
	ms := npt % 1000
	npt /= 1000
	s := npt % 60
	npt /= 60
	m := npt % 60
	npt /= 60
	h := npt
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}

type NPTRange struct {
	Start, End time.Duration
}

func ParseNPTRange(s string) (ret NPTRange, err error) {
	ss := strings.SplitN(s, "-", 2)
	if ss[0] != "" {
		ret.Start, err = ParseNPTTime(ss[0])
		if err != nil {
			return
		}
	}
	if ss[1] != "" {
		ret.End, err = ParseNPTTime(ss[1])
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
