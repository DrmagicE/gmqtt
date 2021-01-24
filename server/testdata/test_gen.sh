#!/usr/bin/env bash
# ca key
openssl genrsa  -out ca-key.pem 2048
# ca certificate
openssl req -new -x509 -days 36500 -key ca-key.pem  -out ca.pem -config openssl.conf
# server key
openssl genrsa -out server-key.pem 2048
# server csr
openssl req -subj "/CN=drmagic.local"  -new -key server-key.pem -out server.csr
# sign the public key with our CA
openssl x509 -req -days 36500  -in server.csr -CA ca.pem -CAkey ca-key.pem \
  -CAcreateserial -out server-cert.pem -extfile extfile.cnf

rm ./ca.srl ./ca-key.pem ./server.csr