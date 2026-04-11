package main

import (
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
)

const maxFramebuf = 1 << 20 // 1 MB cap

type PTYManager struct {
	mu          sync.Mutex
	subscribers map[chan []byte]struct{}
	ptmx        *os.File
	framebuf    []byte
	done        chan struct{}
}

func newPTYManager() *PTYManager {
	return &PTYManager{
		subscribers: make(map[chan []byte]struct{}),
		done:        make(chan struct{}),
	}
}

func (p *PTYManager) stop() {
	select {
	case <-p.done:
		return // already stopped
	default:
		close(p.done)
	}
	p.mu.Lock()
	ptmx := p.ptmx
	p.mu.Unlock()
	if ptmx != nil {
		ptmx.Close()
	}
}

func (p *PTYManager) subscribe() (chan []byte, func()) {
	ch := make(chan []byte, 256)
	p.mu.Lock()
	if len(p.framebuf) > 0 {
		snapshot := make([]byte, len(p.framebuf))
		copy(snapshot, p.framebuf)
		ch <- snapshot
	}
	p.subscribers[ch] = struct{}{}
	p.mu.Unlock()
	return ch, func() {
		p.mu.Lock()
		delete(p.subscribers, ch)
		p.mu.Unlock()
	}
}

func (p *PTYManager) broadcast(data []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.framebuf = append(p.framebuf, data...)
	if len(p.framebuf) > maxFramebuf {
		p.framebuf = p.framebuf[len(p.framebuf)-maxFramebuf:]
	}
	for ch := range p.subscribers {
		select {
		case ch <- data:
		default:
		}
	}
}

func (p *PTYManager) write(data []byte) error {
	p.mu.Lock()
	ptmx := p.ptmx
	p.mu.Unlock()
	if ptmx == nil {
		return nil
	}
	_, err := ptmx.Write(data)
	return err
}

func (p *PTYManager) resize(cols, rows uint16) error {
	p.mu.Lock()
	ptmx := p.ptmx
	p.mu.Unlock()
	if ptmx == nil {
		return nil
	}
	return pty.Setsize(ptmx, &pty.Winsize{Cols: cols, Rows: rows})
}

func (p *PTYManager) start(command string, args []string, workdir string, logger *slog.Logger) {
	for {
		select {
		case <-p.done:
			return
		default:
		}

		p.mu.Lock()
		p.framebuf = nil
		p.mu.Unlock()

		cmd := exec.Command(command, args...)
		if workdir != "" {
			cmd.Dir = workdir
		}
		cmd.Env = append(os.Environ(),
			"TERM=xterm-256color",
			"HOME=/root",
			"CLAUDE_CODE_NO_FLICKER=1",
			"CLAUDE_CODE_DISABLE_MOUSE=1",
		)
		ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 220, Rows: 50})
		if err != nil {
			logger.Error("pty start failed", "err", err)
			select {
			case <-p.done:
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}
		p.mu.Lock()
		p.ptmx = ptmx
		p.mu.Unlock()

		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				p.broadcast(data)
			}
			if err != nil {
				if err != io.EOF {
					logger.Error("pty read error", "err", err)
				}
				break
			}
		}
		ptmx.Close()
		p.mu.Lock()
		p.ptmx = nil
		p.mu.Unlock()
		cmd.Wait()
		logger.Info("process exited, restarting in 2s")
		select {
		case <-p.done:
			return
		case <-time.After(2 * time.Second):
		}
	}
}
