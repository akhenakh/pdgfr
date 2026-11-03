package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/akhenak/pdgfr/internal/wav"
)

// audioPlayer is a streaming player fed raw s16le mono PCM.
type audioPlayer struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
}

// startStreamingPlayer opens the first available raw-PCM player at rate.
func startStreamingPlayer(rate int) (*audioPlayer, error) {
	r := fmt.Sprint(rate)
	candidates := [][]string{
		{"pw-play", "--raw", "--format=s16", "--rate=" + r, "--channels=1", "-"},
		{"pw-cat", "--playback", "--raw", "--format=s16", "--rate=" + r, "--channels=1", "-"},
		{"paplay", "--raw", "--format=s16le", "--rate=" + r, "--channels=1"},
		{"aplay", "-q", "-f", "S16_LE", "-r", r, "-c", "1"},
		// ffplay decodes raw s16le as mono by default. Do not pass -ac:
		// FFmpeg 9 removed it (in favor of -ch_layout) and fails to start.
		{"ffplay", "-f", "s16le", "-ar", r, "-nodisp", "-autoexit", "-loglevel", "quiet", "-"},
	}
	for _, args := range candidates {
		if _, err := exec.LookPath(args[0]); err != nil {
			continue
		}
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		stdin, err := cmd.StdinPipe()
		if err != nil {
			continue
		}
		if err := cmd.Start(); err != nil {
			continue
		}
		return &audioPlayer{cmd: cmd, stdin: stdin}, nil
	}
	return nil, fmt.Errorf("no audio player found (tried pw-play, pw-cat, paplay, aplay, ffplay); write a file with -o instead")
}

func (p *audioPlayer) close() error {
	p.stdin.Close()
	return p.cmd.Wait()
}

// abandon stops a player that failed while receiving audio.
func (p *audioPlayer) abandon() {
	p.stdin.Close()
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	_ = p.cmd.Wait()
}

// canPlayBuffered reports whether the OS has a player that accepts a complete
// WAV file. macOS ships afplay; Windows ships PowerShell and SoundPlayer.
func canPlayBuffered() bool {
	switch runtime.GOOS {
	case "darwin":
		_, err := exec.LookPath("afplay")
		return err == nil
	case "windows":
		return hasCommand("powershell") || hasCommand("pwsh")
	}
	return false
}

func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// playBuffered writes a complete WAV and plays it with the system player.
func playBuffered(audio []float32, rate int) error {
	return playTempWAV(func(f *os.File) error {
		return wav.Write(f, audio, rate)
	})
}

// playBufferedPCM is playBuffered for already-encoded 16-bit PCM.
func playBufferedPCM(pcm []byte, rate int) error {
	return playTempWAV(func(f *os.File) error {
		if _, err := f.Write(wav.Header(rate, len(pcm))); err != nil {
			return err
		}
		_, err := f.Write(pcm)
		return err
	})
}

func playTempWAV(write func(*os.File) error) error {
	f, err := os.CreateTemp("", "pdgfr-*.wav")
	if err != nil {
		return err
	}
	path := f.Name()
	defer os.Remove(path)
	if err := write(f); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("afplay", path)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return cmd.Run()
	case "windows":
		shell, err := exec.LookPath("powershell")
		if err != nil {
			shell, err = exec.LookPath("pwsh")
			if err != nil {
				return fmt.Errorf("PowerShell not found")
			}
		}
		q := strings.ReplaceAll(path, "'", "''")
		// PowerShell 7 needs the assembly loaded; Windows PowerShell 5.1 has it.
		script := fmt.Sprintf("Add-Type -AssemblyName System.Windows.Extensions -ErrorAction SilentlyContinue; $p = New-Object System.Media.SoundPlayer '%s'; $p.PlaySync()", q)
		cmd := exec.Command(shell, "-NoProfile", "-NonInteractive", "-Command", script)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return cmd.Run()
	}
	return fmt.Errorf("buffered playback is not supported on %s", runtime.GOOS)
}
