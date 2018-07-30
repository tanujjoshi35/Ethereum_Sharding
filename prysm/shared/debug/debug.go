// Package debug defines useful profiling utils that came originally with go-ethereum.
// Copyright 2016 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.
package debug

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"runtime/trace"
	"strings"
	"sync"
	"time"

	"github.com/fjl/memsize/memsizeui"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

// Handler is the global debugging handler.
var Handler = new(HandlerT)

// Memsize is the memsizeui Handler(?).
var Memsize memsizeui.Handler
var (
	// PProfFlag to enable pprof HTTP server.
	PProfFlag = cli.BoolFlag{
		Name:  "pprof",
		Usage: "Enable the pprof HTTP server",
	}
	// PProfPortFlag to specify HTTP server listening port.
	PProfPortFlag = cli.IntFlag{
		Name:  "pprofport",
		Usage: "pprof HTTP server listening port",
		Value: 6060,
	}
	// PProfAddrFlag to specify HTTP server address.
	PProfAddrFlag = cli.StringFlag{
		Name:  "pprofaddr",
		Usage: "pprof HTTP server listening interface",
		Value: "127.0.0.1",
	}
	// MemProfileRateFlag to specify the mem profiling rate.
	MemProfileRateFlag = cli.IntFlag{
		Name:  "memprofilerate",
		Usage: "Turn on memory profiling with the given rate",
		Value: runtime.MemProfileRate,
	}
	// CPUProfileFlag to specify where to write the CPU profile.
	CPUProfileFlag = cli.StringFlag{
		Name:  "cpuprofile",
		Usage: "Write CPU profile to the given file",
	}
	// TraceFlag to specify where to write the trace execution profile.
	TraceFlag = cli.StringFlag{
		Name:  "trace",
		Usage: "Write execution trace to the given file",
	}
)

// HandlerT implements the debugging API.
// Do not create values of this type, use the one
// in the Handler variable instead.
type HandlerT struct {
	mu        sync.Mutex
	cpuW      io.WriteCloser
	cpuFile   string
	traceW    io.WriteCloser
	traceFile string
}

// MemStats returns detailed runtime memory statistics.
func (*HandlerT) MemStats() *runtime.MemStats {
	s := new(runtime.MemStats)
	runtime.ReadMemStats(s)
	return s
}

// GcStats returns GC statistics.
func (*HandlerT) GcStats() *debug.GCStats {
	s := new(debug.GCStats)
	debug.ReadGCStats(s)
	return s
}

// CPUProfile turns on CPU profiling for nsec seconds and writes
// profile data to file.
func (h *HandlerT) CPUProfile(file string, nsec uint) error {
	if err := h.StartCPUProfile(file); err != nil {
		return err
	}
	time.Sleep(time.Duration(nsec) * time.Second)
	return h.StopCPUProfile()
}

// StartCPUProfile turns on CPU profiling, writing to the given file.
func (h *HandlerT) StartCPUProfile(file string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cpuW != nil {
		return errors.New("CPU profiling already in progress")
	}
	f, err := os.Create(expandHome(file))
	if err != nil {
		return err
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		if err := f.Close(); err != nil {
			log.Errorf("Failed to close file: %v", err)
		}
		return err
	}
	h.cpuW = f
	h.cpuFile = file
	log.Info("CPU profiling started", "dump", h.cpuFile)
	return nil
}

// StopCPUProfile stops an ongoing CPU profile.
func (h *HandlerT) StopCPUProfile() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	pprof.StopCPUProfile()
	if h.cpuW == nil {
		return errors.New("CPU profiling not in progress")
	}
	log.Info("Done writing CPU profile", "dump", h.cpuFile)
	if err := h.cpuW.Close(); err != nil {
		return err
	}
	h.cpuW = nil
	h.cpuFile = ""
	return nil
}

// GoTrace turns on tracing for nsec seconds and writes
// trace data to file.
func (h *HandlerT) GoTrace(file string, nsec uint) error {
	if err := h.StartGoTrace(file); err != nil {
		return err
	}
	time.Sleep(time.Duration(nsec) * time.Second)
	return h.StopGoTrace()
}

// StartGoTrace turns on tracing, writing to the given file.
func (h *HandlerT) StartGoTrace(file string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.traceW != nil {
		return errors.New("trace already in progress")
	}
	f, err := os.Create(expandHome(file))
	if err != nil {
		return err
	}
	if err := trace.Start(f); err != nil {
		if err := f.Close(); err != nil {
			log.Errorf("Failed to close file: %v", err)
		}
		return err
	}
	h.traceW = f
	h.traceFile = file
	log.Info("Go tracing started", "dump", h.traceFile)
	return nil
}

// StopGoTrace stops an ongoing trace.
func (h *HandlerT) StopGoTrace() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	trace.Stop()
	if h.traceW == nil {
		return errors.New("trace not in progress")
	}
	log.Info("Done writing Go trace", "dump", h.traceFile)
	if err := h.traceW.Close(); err != nil {
		return err
	}
	h.traceW = nil
	h.traceFile = ""
	return nil
}

// BlockProfile turns on goroutine profiling for nsec seconds and writes profile data to
// file. It uses a profile rate of 1 for most accurate information. If a different rate is
// desired, set the rate and write the profile manually.
func (*HandlerT) BlockProfile(file string, nsec uint) error {
	runtime.SetBlockProfileRate(1)
	time.Sleep(time.Duration(nsec) * time.Second)
	defer runtime.SetBlockProfileRate(0)
	return writeProfile("block", file)
}

// SetBlockProfileRate sets the rate of goroutine block profile data collection.
// rate 0 disables block profiling.
func (*HandlerT) SetBlockProfileRate(rate int) {
	runtime.SetBlockProfileRate(rate)
}

// WriteBlockProfile writes a goroutine blocking profile to the given file.
func (*HandlerT) WriteBlockProfile(file string) error {
	return writeProfile("block", file)
}

// MutexProfile turns on mutex profiling for nsec seconds and writes profile data to file.
// It uses a profile rate of 1 for most accurate information. If a different rate is
// desired, set the rate and write the profile manually.
func (*HandlerT) MutexProfile(file string, nsec uint) error {
	runtime.SetMutexProfileFraction(1)
	time.Sleep(time.Duration(nsec) * time.Second)
	defer runtime.SetMutexProfileFraction(0)
	return writeProfile("mutex", file)
}

// SetMutexProfileFraction sets the rate of mutex profiling.
func (*HandlerT) SetMutexProfileFraction(rate int) {
	runtime.SetMutexProfileFraction(rate)
}

// WriteMutexProfile writes a goroutine blocking profile to the given file.
func (*HandlerT) WriteMutexProfile(file string) error {
	return writeProfile("mutex", file)
}

// WriteMemProfile writes an allocation profile to the given file.
// Note that the profiling rate cannot be set through the API,
// it must be set on the command line.
func (*HandlerT) WriteMemProfile(file string) error {
	return writeProfile("heap", file)
}

// Stacks returns a printed representation of the stacks of all goroutines.
func (*HandlerT) Stacks() string {
	buf := new(bytes.Buffer)
	if err := pprof.Lookup("goroutine").WriteTo(buf, 2); err != nil {
		log.Errorf("Failed to write pprof goroutine stacks: %v", err)
	}
	return buf.String()
}

// FreeOSMemory returns unused memory to the OS.
func (*HandlerT) FreeOSMemory() {
	debug.FreeOSMemory()
}

// SetGCPercent sets the garbage collection target percentage. It returns the previous
// setting. A negative value disables GC.
func (*HandlerT) SetGCPercent(v int) int {
	return debug.SetGCPercent(v)
}

func writeProfile(name, file string) error {
	p := pprof.Lookup(name)
	log.Info("Writing profile records", "count", p.Count(), "type", name, "dump", file)
	f, err := os.Create(expandHome(file))
	if err != nil {
		return err
	}
	defer f.Close()
	return p.WriteTo(f, 0)
}

// expands home directory in file paths.
// ~someuser/tmp will not be expanded.
func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~\\") {
		home := os.Getenv("HOME")
		if home == "" {
			if usr, err := user.Current(); err == nil {
				home = usr.HomeDir
			}
		}
		if home != "" {
			p = home + p[1:]
		}
	}
	return filepath.Clean(p)
}

// MigrateFlags sets the global flag from a local flag when it's set.
// This is a temporary function used for migrating old command/flags to the
// new format.
//
// e.g. geth account new --keystore /tmp/mykeystore --lightkdf
//
// is equivalent after calling this method with:
//
// geth --keystore /tmp/mykeystore --lightkdf account new
//
// This allows the use of the existing configuration functionality.
// When all flags are migrated this function can be removed and the existing
// configuration functionality must be changed that is uses local flags
func MigrateFlags(action func(ctx *cli.Context) error) func(*cli.Context) error {
	return func(ctx *cli.Context) error {
		for _, name := range ctx.FlagNames() {
			if ctx.IsSet(name) {
				if err := ctx.GlobalSet(name, ctx.String(name)); err != nil {
					return err
				}
			}
		}
		return action(ctx)
	}
}

// Debug setup and exit functions.

// Setup initializes profiling based on the CLI flags.
// It should be called as early as possible in the program.
func Setup(ctx *cli.Context) error {
	// TODO: Set verbosity level from flag.
	log.SetLevel(log.DebugLevel)

	// profiling, tracing
	runtime.MemProfileRate = ctx.GlobalInt(MemProfileRateFlag.Name)
	if traceFile := ctx.GlobalString(TraceFlag.Name); traceFile != "" {
		if err := Handler.StartGoTrace(TraceFlag.Name); err != nil {
			return err
		}
	}
	if cpuFile := ctx.GlobalString(CPUProfileFlag.Name); cpuFile != "" {
		if err := Handler.StartCPUProfile(cpuFile); err != nil {
			return err
		}
	}

	// pprof server
	if ctx.GlobalBool(PProfFlag.Name) {
		address := fmt.Sprintf("%s:%d", ctx.GlobalString(PProfAddrFlag.Name), ctx.GlobalInt(PProfPortFlag.Name))
		startPProf(address)
	}
	return nil
}

func startPProf(address string) {
	http.Handle("/memsize/", http.StripPrefix("/memsize", &Memsize))
	log.Info("Starting pprof server", "addr", fmt.Sprintf("http://%s/debug/pprof", address))
	go func() {
		if err := http.ListenAndServe(address, nil); err != nil {
			log.Error("Failure in running pprof server", "err", err)
		}
	}()
}

// Exit stops all running profiles, flushing their output to the
// respective file.
func Exit() {
	if err := Handler.StopCPUProfile(); err != nil {
		log.Errorf("Failed to stop CPU profiling: %v", err)
	}
	if err := Handler.StopGoTrace(); err != nil {
		log.Errorf("Failed to stop go tracing: %v", err)
	}
}
