#!/bin/sh
#
# -----------------------------------------------------------
# THIS IS INSECURE
#   USE ONLY IN CI
#   USE ONLY FOR TESTING PURPOSES
# -----------------------------------------------------------
#
# and also for this purpose we really should do better and
# generate saner certs at every run

cat > server.cnf << EOF
[req]
default_md = sha256
prompt = no
req_extensions = v3_ext
distinguished_name = req_distinguished_name

[req_distinguished_name]
CN = localhost

[v3_ext]
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = critical,serverAuth,clientAuth
subjectAltName = DNS:localhost
EOF

openssl req -new -newkey rsa:2048 -keyout ca.key -x509 -sha256 -days 3650 -out ca.crt -nodes
openssl genrsa -out server.key 2048
openssl req -new -key server.key -out server.csr -config server.cnf
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key   -CAcreateserial -out server.crt -days 3650 -sha256 -extfile server.cnf -extensions v3_ext
