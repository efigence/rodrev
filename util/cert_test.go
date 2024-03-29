package util

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

var cert = `
-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQCYjwLYMasAPTs3
7N975NiiPw3CLRlvUju/XcLK8xSLvu9b+zfRSIri4zcM9IUKtGG2EczWbenVL9P+
5CZqm/qCoVTul80hgbWFIFXQja7fIpmJUoMDnasLXFrtS2K2QrJyjiYyLZKAo6jJ
OVxaVUhrzSBPb4mfsKiVbj5kvKuw44sRpdQFDM5qBLzjbxSxyavmGAUV0TvZSCqG
5nWV6vZ4/7z1KocqZ5yzKcvziUOjMFmF4JHqb/55yow0EWpkcj6w5sqU/8rmGCWN
Cs9t5JWJXbIv0NBJLLtXxwRxHLYRt266FsFUmIL+GDsJ4cBnw3nMx+i17erEQDa5
/gOpKXBLAgMBAAECggEACKpsSZuIpcV/TPic4+IRQXtowf7jKE+jf9tI6qj5dyFW
t1gtyNWLY21EqJe7IZXddwTL0zerG1D8Lx5b5Z0L6HUBk2FMEFCDNGysczbubVOp
z63ALhRmrYkxdl6HMj3XpRTYq+uqxrkv0pOk4KdiKxLGeGdS30n0SYiu7Un09rc1
A3uRt8kfqJBbgwpPiUx1omKYeYNdx5cJvIDSsTDHyFpv/wrE0tw1mWzHQv3eUmoV
i8yJf8M6zf/DKlfwH9NZixPw9X4/BiUw2UEzHTvxl37wVewlsK4IXAAl2k9xbAVy
2BGdLncBZKqqm9qFkOeoyyGiingdHebtrKaWWYgzAQKBgQDLzTq/n/sjIOb/KX+R
khFn8indUujB4YbdujaBtQiDy7ZqTo05W+fIe7hBOhGmDtETghvq/ZXnvQlnmUzn
Z8uFQ/IqCBdIBJnPSp4dVU2e6tzG5vjBtqWp9YT/M9V9V34waEmg+Du3TWqtFs/z
rhDKoBaWqCkLXrVMz9L7FT9JfQKBgQC/oecN7dv6Bf9i5wOBxoFTEc471Yg//9v7
gGh9RpsLuN7BIL1QK5X8iisTiucFWpRF+hbPKCAlsKh9ZkpkvsvbRp5YBCkeFgR1
LKUPXIFntk6AOPLD+JkmsxgVBkQ21rmwL8CphkCQkoH1qV1BTbGmjBE0nSyC7Pdp
qyigaLyLZwKBgBvXYJQ3Db7hyszG5YeEdd5GZdO3dNavsZXuz21uxsgCd1wUeRSB
6qsnw6cdgLW3xnYsyxWSKP1shLFjbu8Z7eK5woZdbpjHECASaIFHWwA/w4NkoE0O
X5lVUiLu+NZTsoh4Zr6XysiiydT1jdkTSXX04sEYHetQt+HTblYgs/GdAoGBALPN
QhDUs8iEphnzLVwvl5oMo9eKQ7vg4lO5KNEaVVGLVR4aDObS7ni0XzSH6LkiQurh
e1NFj9wtQ/nc50JdrsKAIQPua3H6MJaMnBasshJQgJlYVZfAglpIQflAFvLpR/Li
6z9kYuIDRQLttT0Xm+7rjx0xt9jkZEP2Pzk67GVTAoGAGWcfCQOhuTehZPKHhZ+X
76Vi5jwH06Q7aiGQIdIcLkg9YopgjQ1sMaFKWf3MtvH3CZVHo3gDDo16FhYZNyqy
lrm7EgZaYIlDDFdCyPhufMXn97EnBESFetYz908rLqFTghncVD98yutqKHZX1LPR
d7Oj0bsS5vO4p/WoPLPdrGY=
-----END PRIVATE KEY-----
-----BEGIN CERTIFICATE-----
MIIDozCCAougAwIBAgIUcASgKgtOTs32v0l4qolF0aaOPwMwDQYJKoZIhvcNAQEL
BQAwYDELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM
GEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDEZMBcGA1UEAwwQY2VydC5leGFtcGxl
LmNvbTAgFw0yMzEwMTcxNDA5NDlaGA8yMTIzMDkyMzE0MDk0OVowYDELMAkGA1UE
BhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoMGEludGVybmV0IFdp
ZGdpdHMgUHR5IEx0ZDEZMBcGA1UEAwwQY2VydC5leGFtcGxlLmNvbTCCASIwDQYJ
KoZIhvcNAQEBBQADggEPADCCAQoCggEBAJiPAtgxqwA9Ozfs33vk2KI/DcItGW9S
O79dwsrzFIu+71v7N9FIiuLjNwz0hQq0YbYRzNZt6dUv0/7kJmqb+oKhVO6XzSGB
tYUgVdCNrt8imYlSgwOdqwtcWu1LYrZCsnKOJjItkoCjqMk5XFpVSGvNIE9viZ+w
qJVuPmS8q7DjixGl1AUMzmoEvONvFLHJq+YYBRXRO9lIKobmdZXq9nj/vPUqhypn
nLMpy/OJQ6MwWYXgkepv/nnKjDQRamRyPrDmypT/yuYYJY0Kz23klYldsi/Q0Eks
u1fHBHEcthG3broWwVSYgv4YOwnhwGfDeczH6LXt6sRANrn+A6kpcEsCAwEAAaNT
MFEwHQYDVR0OBBYEFK2qa6UNtIFvJagPGHSpk0zFEf/zMB8GA1UdIwQYMBaAFK2q
a6UNtIFvJagPGHSpk0zFEf/zMA8GA1UdEwEB/wQFMAMBAf8wDQYJKoZIhvcNAQEL
BQADggEBAHjXaii6492xReTw9IpXcGEwwp1oYAhhkDp4T2PXPKeOnqcuTqQwpCi8
OXonILdlFkHNlZ2AheaX4AFvlzReDCjitMSZFhrqh0Chiimtffhfu6+DM0YH9kUy
1BTy5hza/ebvuHYPsK0vhJ2YJF7YAaOFMEsBI3iOBnxvUam1em/JdU5o8bt+oFqX
hAJ59q4oBmRL827//4dy8b8q2uziqaNpM6r8Vf9YIygKFY2qFwZI8YkIHPWFy+78
eTcvlDdT0C50uz39d5Sc1IouCiwpdKNiLJtCxjzExRfwAhXZLBzCD1DEnjUjiQeH
/Vkd6+WeZw/njUb2Lk0iKCTZ4Motheg=
-----END CERTIFICATE-----
`

func TestGetCNFromCert(t *testing.T) {
	cn := GetCNFromCert([]byte(cert))
	assert.Equal(t, "cert.example.com", cn)
}
