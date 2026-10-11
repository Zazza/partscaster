package config

import (
	"errors"
	"net"
	"strings"
)

// DefaultWorkerPort — порт воркера по умолчанию (uvicorn в юните systemd).
const DefaultWorkerPort = "8091"

// NormalizeURL — адрес воркера, как его ввёл человек, в полный URL: «192.168.1.5» →
// «http://192.168.1.5:8091», «host:9000» → «http://host:9000». Явная схема — как введено
// (порт не дописывается: https://… может стоять за прокси на 443). Хвостовой «/» убирается.
func NormalizeURL(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("адрес воркера пуст")
	}
	if !strings.Contains(s, "://") {
		host := strings.TrimRight(s, "/")
		if _, _, err := net.SplitHostPort(host); err != nil {
			host = net.JoinHostPort(host, DefaultWorkerPort)
		}
		s = "http://" + host
	}
	return strings.TrimRight(s, "/"), nil
}
