package main

import (
	"archive/tar"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// A control plane that was handed over gives its last snapshot and the
// cluster's keys to the one machine named to take over, and to nobody else:
// both ends show a certificate of the cluster's own CA, this one the API
// server's, the other the kubelet's of the node that was named. No person
// carries the keys from one machine to the other.

// snapshotServe ADDRESS NODE DIR PKI: serves the newest etcd-*.db and pki.tar
// of DIR at https://ADDRESS/ to the kubelet of NODE, until it is stopped.
func snapshotServe(address, node, dir, pki string) {
	ca, err := os.ReadFile(filepath.Join(pki, "ca.crt"))
	if err != nil {
		log.Fatal(err)
	}
	nodes := x509.NewCertPool()
	nodes.AppendCertsFromPEM(ca)
	server := &http.Server{
		Addr:      address,
		TLSConfig: &tls.Config{ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: nodes, MinVersion: tls.VersionTLS12},
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if who := r.TLS.PeerCertificates[0].Subject.CommonName; who != "system:node:"+node {
				log.Printf("refused %s: the control plane was handed over to %s", who, node)
				http.Error(w, "this control plane was handed over to another node", http.StatusForbidden)
				return
			}
			snapshots, _ := filepath.Glob(filepath.Join(dir, "etcd-*.db"))
			if len(snapshots) == 0 {
				http.Error(w, "no snapshot", http.StatusServiceUnavailable)
				return
			}
			sort.Strings(snapshots) // their names are their times
			archive := tar.NewWriter(w)
			for _, file := range []string{snapshots[len(snapshots)-1], filepath.Join(dir, "pki.tar")} {
				if err := addFile(archive, file); err != nil {
					log.Printf("send %s: %v", file, err)
					return // the stream ends without its trailer: the other end sees it
				}
			}
			if archive.Close() == nil {
				log.Printf("%s and the cluster's keys were taken by %s", filepath.Base(snapshots[len(snapshots)-1]), node)
			}
		}),
	}
	log.Fatal(server.ListenAndServeTLS(filepath.Join(pki, "apiserver.crt"), filepath.Join(pki, "apiserver.key")))
}

func addFile(archive *tar.Writer, file string) error {
	from, err := os.Open(file)
	if err != nil {
		return err
	}
	defer from.Close()
	info, err := from.Stat()
	if err != nil {
		return err
	}
	if err := archive.WriteHeader(&tar.Header{Name: filepath.Base(file), Mode: 0600, Size: info.Size(), ModTime: info.ModTime()}); err != nil {
		return err
	}
	_, err = io.Copy(archive, from)
	return err
}

// snapshotFetch ADDRESS NAME DIR: this node takes the snapshot and the keys
// from the control plane at ADDRESS, which shows the certificate of NAME,
// into DIR, with its kubelet's certificate.
func snapshotFetch(address, name, dir string) error {
	ca, err := os.ReadFile("/etc/kubernetes/pki/ca.crt")
	if err != nil {
		return fmt.Errorf("this machine is no node of the cluster: %w", err)
	}
	cluster := x509.NewCertPool()
	cluster.AppendCertsFromPEM(ca)
	const kubelet = "/var/lib/kubelet/pki/kubelet-client-current.pem"
	own, err := tls.LoadX509KeyPair(kubelet, kubelet)
	if err != nil {
		return fmt.Errorf("this node's kubelet certificate: %w", err)
	}
	client := &http.Client{Timeout: 10 * time.Minute, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: cluster, ServerName: name, Certificates: []tls.Certificate{own}, MinVersion: tls.VersionTLS12}}}
	response, err := client.Get("https://" + address + "/")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		answer, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("%s: %s", response.Status, answer)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	archive, files := tar.NewReader(response.Body), 0
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("the snapshot ended early: %w", err)
		}
		to, err := os.OpenFile(filepath.Join(dir, filepath.Base(header.Name)), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		_, err = io.Copy(to, archive)
		if closed := to.Close(); err == nil {
			err = closed
		}
		if err != nil {
			return fmt.Errorf("the snapshot ended early: %w", err)
		}
		files++
	}
	if files != 2 {
		return fmt.Errorf("the snapshot ended early: %d of its 2 files", files)
	}
	return nil
}
