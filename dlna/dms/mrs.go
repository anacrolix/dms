package dms

import (
	"net/http"

	"github.com/anacrolix/dms/upnp"
)

type mediaReceiverRegistrarService struct {
	*Server
	upnp.Eventing
}

func (me *mediaReceiverRegistrarService) Handle(action string, argsXML []byte, r *http.Request) (map[string]string, error) {
//	host := r.Host
//	userAgent := r.UserAgent()
	switch action {
	case "IsAuthorized":
		return map[string]string{
			"Result": "1",
		}, nil
	case "RegisterDevice":
		return nil, nil
	case "IsValidated":
		return map[string]string{
			"Result": "1",
		}, nil
	default:
		return nil, upnp.InvalidActionError
	}
}
