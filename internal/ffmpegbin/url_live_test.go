package ffmpegbin

import (
	"net/http"
	"os"
	"testing"
	"time"
)

// Спецификация (карточка internal-ffmpeg-pin, ТК1): проверка живости закреплённой ссылки на
// сборку ffmpeg. Без сети go test ./... не должен краснеть — тест включается переменной
// PARTSCASTER_NET_TEST=1 (CI: еженедельно, при изменении internal/ffmpegbin, вручную).
// Красный тест сам объясняет причину и что делать.

const pinHelp = "ссылка на закреплённую сборку ffmpeg (BtbN FFmpeg-Builds) умерла или изменилась.\n" +
	"Почему: дневные сборки BtbN удаляются примерно через 2 недели, месячные " +
	"(autobuild-YYYY-MM-<последний день месяца>) живут около 2 лет.\n" +
	"Что сделать: взять месячную сборку win64-gpl-shared той же ветки на " +
	"https://github.com/BtbN/FFmpeg-Builds/releases и обновить URL, SHA256 и Size " +
	"в internal/ffmpegbin/ffmpegbin.go (SHA256 и Size — по скачанному архиву)."

func TestPinnedURLAlive(t *testing.T) {
	if os.Getenv("PARTSCASTER_NET_TEST") != "1" {
		t.Skip("проверка сети: ссылка на сборку ffmpeg проверяется только с PARTSCASTER_NET_TEST=1")
	}
	client := &http.Client{Timeout: 60 * time.Second} // редиректы GitHub → CDN клиент проходит сам
	req, err := http.NewRequest(http.MethodHead, URL, nil)
	if err != nil {
		t.Fatalf("%s\nURL: %s\nошибка запроса: %v", pinHelp, URL, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s\nURL: %s\nошибка сети: %v", pinHelp, URL, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s\nURL: %s\nкод ответа: %d (ожидался 200)", pinHelp, URL, resp.StatusCode)
	}
	if resp.ContentLength != int64(Size) {
		t.Fatalf("%s\nURL: %s\nразмер по ответу: %d байт, в коде Size = %d", pinHelp, URL, resp.ContentLength, Size)
	}
}
