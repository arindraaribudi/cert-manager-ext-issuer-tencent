package tencent

import "time"

type Certificate struct {
	ID        string
	Name      string
	Status    string
	NotBefore time.Time
	NotAfter  time.Time
}

type DownloadedCert struct {
	Certificates []byte
	Chain        []byte
	PrivateKey   []byte
}
