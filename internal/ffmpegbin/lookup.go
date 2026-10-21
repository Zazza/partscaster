package ffmpegbin

import "os/exec"

// execLookPath — ffmpeg в PATH (обёртка для тестов).
var execLookPath = func() (string, error) { return exec.LookPath("ffmpeg") }
