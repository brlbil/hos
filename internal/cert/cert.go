// SPDX-License-Identifier: MIT

// Package cert provides TLS certificate generation and management for HOS servers.
// It handles CA certificate creation and server certificate generation.
package cert

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// CreateCA creates or loads a Certificate Authority certificate and key
func CreateCA(path string) (tls.Certificate, []byte, error) {
	dir := filepath.Join(path, ".certs")
	caFilePath := filepath.Join(dir, "ca.pem")
	keyFilePath := filepath.Join(dir, "key.pem")

	// check certificate already exists??
	if cert, err := tls.LoadX509KeyPair(caFilePath, keyFilePath); err == nil {
		certBytes, err := os.ReadFile(caFilePath)
		if err != nil {
			return tls.Certificate{}, nil, err
		}
		return cert, certBytes, nil
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return tls.Certificate{}, nil, err
	}

	// create the private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	// set up our CA certificate
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Home Object Store"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	caCertBytes, err := x509.CreateCertificate(rand.Reader, template, template, privateKey.Public(), privateKey)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	caFile, err := os.Create(caFilePath)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	defer caFile.Close()

	caCertBuffer := &bytes.Buffer{}
	caCertWriter := io.MultiWriter(caFile, caCertBuffer)
	if err := pem.Encode(caCertWriter, &pem.Block{Type: "CERTIFICATE", Bytes: caCertBytes}); err != nil {
		return tls.Certificate{}, nil, err
	}

	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	keyFile, err := os.Create(keyFilePath)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	defer keyFile.Close()

	privateKeyBuffer := &bytes.Buffer{}
	privateKeyWriter := io.MultiWriter(keyFile, privateKeyBuffer)
	if err := pem.Encode(privateKeyWriter, &pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyBytes}); err != nil {
		return tls.Certificate{}, nil, err
	}

	cert, err := tls.X509KeyPair(caCertBuffer.Bytes(), privateKeyBuffer.Bytes())
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	return cert, caCertBuffer.Bytes(), nil
}

// CreateServerCert generates a server certificate signed by the CA
func CreateServerCert(caCert tls.Certificate, dnsNames ...string) (*tls.Config, error) {
	// create the private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, err
	}

	dnsNames = append(dnsNames, "localhost")
	hostname, err := os.Hostname()
	if err == nil {
		dnsNames = append(dnsNames, hostname)
	}

	// set up server certificate
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Home Object Store"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 6, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		DNSNames:              dnsNames,
		BasicConstraintsValid: true,
	}

	interfaceAddrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, addr := range interfaceAddrs {
			if ipNetwork, ok := addr.(*net.IPNet); ok {
				if ipAddress := ipNetwork.IP.To4(); ipAddress != nil {
					template.IPAddresses = append(template.IPAddresses, ipAddress)
				}
			}
		}
	}

	caCertificate, err := x509.ParseCertificate(caCert.Certificate[0])
	if err != nil {
		return nil, err
	}
	serverCertBytes, err := x509.CreateCertificate(rand.Reader, template, caCertificate, privateKey.Public(), caCert.PrivateKey)
	if err != nil {
		return nil, err
	}

	serverCertBuffer := &bytes.Buffer{}
	if err := pem.Encode(serverCertBuffer, &pem.Block{Type: "CERTIFICATE", Bytes: serverCertBytes}); err != nil {
		return nil, err
	}

	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, err
	}

	serverKeyBuffer := &bytes.Buffer{}
	if err := pem.Encode(serverKeyBuffer, &pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyBytes}); err != nil {
		return nil, err
	}

	serverCertificate, err := tls.X509KeyPair(serverCertBuffer.Bytes(), serverKeyBuffer.Bytes())
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	certPool.AddCert(caCertificate)

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP521,
			tls.CurveP384,
			tls.CurveP256,
		},
		PreferServerCipherSuites: true,
		Certificates:             []tls.Certificate{serverCertificate},
		ClientCAs:                certPool,
	}

	return tlsConfig, nil
}
