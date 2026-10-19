package workerexe

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"testing"
)

// Карточка internal-win-tray, усл. 2c и 3: окно установки берёт рекомендацию и размеры моделей из engine_cpp.py
// вшитого архива воркера — до его распаковки и до установки Python (тесты помощников написал исполнитель:
// разбор текста и архива, не новая логика карточки).

func TestModelSizesFromRealEngine(t *testing.T) {
	b, err := os.ReadFile("../../worker/engine_cpp.py")
	if err != nil {
		t.Fatal(err)
	}
	got := ModelSizesFromBytes(b)
	for name, want := range map[string]int64{"bf16": 7166072352, "q8": 3810232064, "q6": 2943331072, "q5": 2622936832} {
		if got[name] != want {
			t.Errorf("размер %s = %d, ждали %d (MODELS в engine_cpp.py)", name, got[name], want)
		}
	}
	if len(got) != 4 {
		t.Errorf("моделей %d, ждали 4: VAE в список не входит: %v", len(got), got)
	}
}

func TestRecommendFromBytesSameAsSource(t *testing.T) {
	b, err := os.ReadFile("../../worker/engine_cpp.py")
	if err != nil {
		t.Fatal(err)
	}
	for _, mib := range []int{0, 8188, 10240, 12282, 16376} {
		m1, l1 := RecommendFromBytes(b, mib)
		m2, l2 := RecommendFromSource("../../worker", mib)
		if m1 != m2 || l1 != l2 {
			t.Errorf("%d МиБ: по тексту %s/%v, по файлу %s/%v", mib, m1, l1, m2, l2)
		}
	}
}

func tarGz(t *testing.T, files map[string]string) *bytes.Reader {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(buf.Bytes())
}

func TestReadArchiveFile(t *testing.T) {
	arch := tarGz(t, map[string]string{"worker-1.0/engine_cpp.py": "RECOMMEND_Q8_MIB = 9728\n", "worker-1.0/x.py": "x"})
	b, err := ReadArchiveFile(arch, "engine_cpp.py")
	if err != nil || string(b) != "RECOMMEND_Q8_MIB = 9728\n" {
		t.Fatalf("ReadArchiveFile = %q, %v", b, err)
	}
	if _, err := ReadArchiveFile(tarGz(t, map[string]string{"worker-1.0/x.py": "x"}), "engine_cpp.py"); err == nil {
		t.Error("файла нет в архиве — ждали ошибку")
	}
	if _, err := ReadArchiveFile(bytes.NewReader([]byte("не архив")), "engine_cpp.py"); err == nil {
		t.Error("не gzip — ждали ошибку")
	}
}
