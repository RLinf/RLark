// stationstub stands in for Station API's bootstrap endpoint in the isolated
// run of hack/e2e/control-plane.sh, which has no management plane: the
// installer redeems its code here. A code's answer is the file
// <dir>/answers/<code>.json, written by the run once it has seen the
// machine's public key in <dir>/redeem/<code>.key; until then the installer
// is told to wait, as Station API tells it while station-controller computes.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

func main() {
	addr := flag.String("addr", ":18443", "")
	cert := flag.String("tls-cert-file", "", "")
	key := flag.String("tls-key-file", "", "")
	dir := flag.String("dir", "/data", "")
	flag.Parse()
	code := regexp.MustCompile(`^[a-z0-9]{8,32}$`)
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	http.HandleFunc("/v1/bootstrap/redeem", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Code      string `json:"code"`
			PublicKey string `json:"public_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || !code.MatchString(request.Code) {
			http.Error(w, `{"state":"rejected","reason":"unknown code"}`, http.StatusNotFound)
			return
		}
		if err := os.WriteFile(filepath.Join(*dir, "redeem", request.Code+".key"), []byte(request.PublicKey+"\n"), 0644); err != nil {
			log.Print(err)
		}
		w.Header().Set("Content-Type", "application/json")
		answer, err := os.ReadFile(filepath.Join(*dir, "answers", request.Code+".json"))
		if err != nil {
			w.WriteHeader(http.StatusAccepted)
			w.Write([]byte(`{"state":"pending","reason":"waiting for the run to compute the tunnel"}`))
			return
		}
		w.Write(answer)
	})
	log.Fatal(http.ListenAndServeTLS(*addr, *cert, *key, nil))
}
