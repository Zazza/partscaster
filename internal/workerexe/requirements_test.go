package workerexe

import (
	"os"
	"strings"
	"testing"
)

// Карточка internal-fix-found, условие 1: установка воркера на Linux ставит yue2-infer из архива
// того же закреплённого коммита YuE, что и программа воркера Windows (YueCommit).
func TestRequirementsPinYue2InferToYueCommit(t *testing.T) {
	data, err := os.ReadFile("../../worker/requirements.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := "yue2-infer @ https://github.com/multimodal-art-projection/YuE/archive/" + YueCommit + ".zip"
	var found []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "yue2-infer") {
			found = append(found, line)
		}
	}
	if len(found) != 1 || found[0] != want {
		t.Errorf("worker/requirements.txt: строки yue2-infer = %q, want ровно одна %q", found, want)
	}
}
