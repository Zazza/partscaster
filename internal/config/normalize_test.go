package config

import (
	"testing"
)

// Спецификация нормализации адреса воркера:
//   - без схемы и порта → http://<host>:8091;
//   - без схемы, с портом → http://<host>:<port>;
//   - явная схема (http/https) — как введено, порт не дописывается;
//   - хвостовой «/» убирается, пробелы по краям убираются;
//   - пусто (в том числе одни пробелы) → ошибка.

func TestNormalizeURL(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"192.168.1.5", "http://192.168.1.5:8091"},
		{"host", "http://host:8091"},
		{"localhost", "http://localhost:8091"},
		{"gpu-box.lan", "http://gpu-box.lan:8091"},
		{"host:9000", "http://host:9000"},
		{"192.168.1.5:8091", "http://192.168.1.5:8091"},
		{"https://h", "https://h"},
		{"http://h", "http://h"},
		{"http://h:8091", "http://h:8091"},
		{"https://h:443", "https://h:443"},
		{"http://h:8091/", "http://h:8091"},
		{"https://h/", "https://h"},
		{"host/", "http://host:8091"},
		{"host:9000/", "http://host:9000"},
		{"  host  ", "http://host:8091"},
		{"\thttp://h:8091/ \n", "http://h:8091"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeURL(c.in)
			if err != nil {
				t.Fatalf("NormalizeURL(%q): unexpected error %v", c.in, err)
			}
			if got != c.want {
				t.Fatalf("NormalizeURL(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestNormalizeURLEmptyIsError(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"", "   ", "\t\n"} {
		if got, err := NormalizeURL(in); err == nil {
			t.Errorf("NormalizeURL(%q) = %q, want error", in, got)
		}
	}
}

// Нормализованный адрес — неподвижная точка: повторная нормализация его не меняет
// (адрес из настроек можно пропускать через функцию сколько угодно раз).
func TestNormalizeURLIdempotent(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"192.168.1.5", "host:9000", "https://h/", " http://h "} {
		once, err := NormalizeURL(in)
		if err != nil {
			t.Fatalf("NormalizeURL(%q): %v", in, err)
		}
		twice, err := NormalizeURL(once)
		if err != nil || twice != once {
			t.Errorf("NormalizeURL(%q) = %q, again = %q (err %v)", in, once, twice, err)
		}
	}
}
