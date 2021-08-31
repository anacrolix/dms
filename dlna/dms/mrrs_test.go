package dms

import (
	"strings"
	"testing"
)

// Check that the X_MS_MediaReceiverRegistrar is faked out properly.
func TestMediaReceiverRegistrarService(t *testing.T) {
	env := soap.Envelope{
		Body: soap.Body{
			Action: []byte("RegisterDevice"),
		},
	}
	req, err := http.NewRequest("POST", testURL+"ctl", bytes.NewReader(mustMarshalXML(env)))
	require.NoError(t, err)
	req.Header.Set("SOAPACTION", `"urn:microsoft.com:service:X_MS_MediaReceiverRegistrar:1#RegisterDevice"`)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "<RegistrationRespMsg>")
}
