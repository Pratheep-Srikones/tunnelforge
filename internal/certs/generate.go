//go:build ignore

package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"log"
	"math/big"
	"net"
	"os"
	"strings"
	"time"
)

func main() {
	var hostFlag string
	flag.StringVar(&hostFlag, "host", "", `Comma-separated list of hostnames and IP addresses for the server certificate\nExample: -host "10.10.23.45,tunnel.mydomain.com"`)
	flag.Parse()

	if hostFlag == "" {
		hostFlag = os.Getenv("FORGE_TLS_HOSTS")
	}

	dnsNames := []string{"localhost"}
	ipAddresses := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}

	if hostFlag != "" {
		for _, h := range strings.Split(hostFlag, ",") {
			h = strings.TrimSpace(h)
			if h == "" {
				continue
			}
			if ip := net.ParseIP(h); ip != nil {
				ipAddresses = append(ipAddresses, ip)
			} else {
				dnsNames = append(dnsNames, h)
			}
		}
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2026),
		Subject: pkix.Name{
			Organization: []string{"TunnelForge"},
			Country:      []string{"LK"},
			Province:     []string{"Northern"},
			Locality:     []string{"Jaffna"},
			CommonName:   "TunnelForge CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(3, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	caPrivateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf("Failed to generate private key: %v", err)
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caPrivateKey.PublicKey, caPrivateKey)
	if err != nil {
		log.Fatalf("Failed to create CA certificate: %v", err)
	}

	caCertFile, err := os.Create("ca.crt")
	if err != nil {
		log.Fatalf("Failed to create ca.crt: %v", err)
	}
	defer caCertFile.Close()

	err = pem.Encode(caCertFile, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})
	if err != nil {
		log.Fatalf("Failed to write PEM certificate: %v", err)
	}

	caKeyFile, err := os.OpenFile("ca.key", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Failed to create ca.key: %v", err)
	}
	defer caKeyFile.Close()

	keyBytes, err := x509.MarshalECPrivateKey(caPrivateKey)
	if err != nil {
		log.Fatalf("Failed to marshal ECDSA private key: %v", err)
	}

	pem.Encode(caKeyFile, &pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})

	log.Println("Successfully generated ca.crt and ca.key!")

	// ==============================================================

	serverPrivateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf("Failed to generate server private key: %v", err)
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2027),
		Subject: pkix.Name{
			Organization: []string{"TunnelForge"},
			CommonName:   "TunnelForge Server",
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(2, 0, 0),
		IsCA:        false,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		DNSNames:    dnsNames,
		IPAddresses: ipAddresses,
	}

	serverBytes, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverPrivateKey.PublicKey, caPrivateKey)
	if err != nil {
		log.Fatalf("Failed to create server certificate: %v", err)
	}

	serverCertFile, err := os.Create("server.crt")
	if err != nil {
		log.Fatalf("Failed to create server.crt: %v", err)
	}
	defer serverCertFile.Close()
	pem.Encode(serverCertFile, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: serverBytes,
	})

	serverKeyFile, err := os.OpenFile("server.key", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Failed to create server.key: %v", err)
	}
	defer serverKeyFile.Close()
	serverKeyBytes, err := x509.MarshalECPrivateKey(serverPrivateKey)
	if err != nil {
		log.Fatalf("Failed to marshal server key: %v", err)
	}
	pem.Encode(serverKeyFile, &pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: serverKeyBytes,
	})

	log.Println("Successfully generated server.crt and server.key signed by TunnelForge CA!")

}
