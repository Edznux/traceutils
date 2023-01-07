package parser

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
)

type Event struct {
	Type EventType
	Args []uint64
	Str  []byte
}

type Parser struct {
	in   *bufio.Reader
	br   bytes.Reader
	args []byte // scratch buf
}

func NewParser(r io.Reader) (*Parser, error) {
	p := &Parser{in: bufio.NewReader(r)}
	return p, p.header()
}

var header = func() []byte {
	header := make([]byte, 16)
	// only supporting go 1.19 traces for now
	copy(header, "go 1.19 trace")
	return header
}()

// header reads the header and returns an error if it is invalid.
func (p *Parser) header() error {
	buf := make([]byte, len(header))
	_, err := io.ReadFull(p.in, buf)
	if err != nil {
		return err
	} else if !bytes.Equal(buf, header) {
		return fmt.Errorf("invalid header: %q", string(buf))
	}
	return nil
}

// Parse parses an event or returns an error.
func (p *Parser) Parse(e *Event) error {
	b, err := p.in.ReadByte()
	if err != nil {
		return err
	}

	// 0x3f is 00111111 in binary
	e.Type = EventType(b & 0x3f)
	e.Args = e.Args[:0]
	e.Str = e.Str[:0]
	p.args = p.args[:0]

	narg := b>>6 + 1
	if e.Type == EventString {
		id, err := readVal(p.in)
		if err != nil {
			return err
		}
		e.Args = append(e.Args, id)
		length, err := readVal(p.in)
		if err != nil {
			return err
		}
		for i := uint64(0); i < length; i++ {
			e.Str = append(e.Str, 0)
		}

		// read string into e.Str
		if _, err := io.ReadFull(p.in, e.Str); err != nil {
			return err
		}
	} else if narg < 4 {
		// inlined arguments
		for i := 0; i < int(narg); i++ {
			arg, err := readVal(p.in)
			if err != nil {
				return err
			}
			e.Args = append(e.Args, arg)
		}
	} else {
		length, err := readVal(p.in)
		if err != nil {
			return err
		}
		for i := uint64(0); i < length; i++ {
			p.args = append(p.args, 0)
		}
		_, err = io.ReadFull(p.in, p.args)
		if err != nil {
			return err
		}
		p.br.Reset(p.args)
		for {
			arg, err := readVal(&p.br)
			if err == io.EOF {
				break
			} else if err != nil {
				return err
			}
			e.Args = append(e.Args, arg)
		}
	}

	if e.Type == EventUserLog {
		length, err := readVal(p.in)
		if err != nil {
			return err
		}
		for i := uint64(0); i < length; i++ {
			e.Str = append(e.Str, 0)
		}
		if _, err := io.ReadFull(p.in, e.Str); err != nil {
			return err
		}

	}

	return nil
}

// readVal reads a base-128 varint encoded value from an io.Reader.
func readVal(r io.ByteReader) (uint64, error) {
	var val uint64
	var shift uint
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		val |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			break
		}
		shift += 7
	}
	return val, nil
}

//go:generate stringer -type=EventType

type EventType byte

// Event types in the trace, args are given in square brackets.
const (
	EventNone              EventType = 0  // unused
	EventBatch             EventType = 1  // start of per-P batch of events [pid, timestamp]
	EventFrequency         EventType = 2  // contains tracer timer frequency [frequency (ticks per second)]
	EventStack             EventType = 3  // stack [stack id, number of PCs, array of {PC, func string ID, file string ID, line}]
	EventGomaxprocs        EventType = 4  // current value of GOMAXPROCS [timestamp, GOMAXPROCS, stack id]
	EventProcStart         EventType = 5  // start of P [timestamp, thread id]
	EventProcStop          EventType = 6  // stop of P [timestamp]
	EventGCStart           EventType = 7  // GC start [timestamp, seq, stack id]
	EventGCDone            EventType = 8  // GC done [timestamp]
	EventGCSTWStart        EventType = 9  // GC STW start [timestamp, kind]
	EventGCSTWDone         EventType = 10 // GC STW done [timestamp]
	EventGCSweepStart      EventType = 11 // GC sweep start [timestamp, stack id]
	EventGCSweepDone       EventType = 12 // GC sweep done [timestamp, swept, reclaimed]
	EventGoCreate          EventType = 13 // goroutine creation [timestamp, new goroutine id, new stack id, stack id]
	EventGoStart           EventType = 14 // goroutine starts running [timestamp, goroutine id, seq]
	EventGoEnd             EventType = 15 // goroutine ends [timestamp]
	EventGoStop            EventType = 16 // goroutine stops (like in select{}) [timestamp, stack]
	EventGoSched           EventType = 17 // goroutine calls Gosched [timestamp, stack]
	EventGoPreempt         EventType = 18 // goroutine is preempted [timestamp, stack]
	EventGoSleep           EventType = 19 // goroutine calls Sleep [timestamp, stack]
	EventGoBlock           EventType = 20 // goroutine blocks [timestamp, stack]
	EventGoUnblock         EventType = 21 // goroutine is unblocked [timestamp, goroutine id, seq, stack]
	EventGoBlockSend       EventType = 22 // goroutine blocks on chan send [timestamp, stack]
	EventGoBlockRecv       EventType = 23 // goroutine blocks on chan recv [timestamp, stack]
	EventGoBlockSelect     EventType = 24 // goroutine blocks on select [timestamp, stack]
	EventGoBlockSync       EventType = 25 // goroutine blocks on Mutex/RWMutex [timestamp, stack]
	EventGoBlockCond       EventType = 26 // goroutine blocks on Cond [timestamp, stack]
	EventGoBlockNet        EventType = 27 // goroutine blocks on network [timestamp, stack]
	EventGoSysCall         EventType = 28 // syscall enter [timestamp, stack]
	EventGoSysExit         EventType = 29 // syscall exit [timestamp, goroutine id, seq, real timestamp]
	EventGoSysBlock        EventType = 30 // syscall blocks [timestamp]
	EventGoWaiting         EventType = 31 // denotes that goroutine is blocked when tracing starts [timestamp, goroutine id]
	EventGoInSyscall       EventType = 32 // denotes that goroutine is in syscall when tracing starts [timestamp, goroutine id]
	EventHeapAlloc         EventType = 33 // gcController.heapLive change [timestamp, heap_alloc]
	EventHeapGoal          EventType = 34 // gcController.heapGoal() (formerly next_gc) change [timestamp, heap goal in bytes]
	EventTimerGoroutine    EventType = 35 // not currently used; previously denoted timer goroutine [timer goroutine id]
	EventFutileWakeup      EventType = 36 // denotes that the previous wakeup of this goroutine was futile [timestamp]
	EventString            EventType = 37 // string dictionary entry [ID, length, string]
	EventGoStartLocal      EventType = 38 // goroutine starts running on the same P as the last event [timestamp, goroutine id]
	EventGoUnblockLocal    EventType = 39 // goroutine is unblocked on the same P as the last event [timestamp, goroutine id, stack]
	EventGoSysExitLocal    EventType = 40 // syscall exit on the same P as the last event [timestamp, goroutine id, real timestamp]
	EventGoStartLabel      EventType = 41 // goroutine starts running with label [timestamp, goroutine id, seq, label string id]
	EventGoBlockGC         EventType = 42 // goroutine blocks on GC assist [timestamp, stack]
	EventGCMarkAssistStart EventType = 43 // GC mark assist start [timestamp, stack]
	EventGCMarkAssistDone  EventType = 44 // GC mark assist done [timestamp]
	EventUserTaskCreate    EventType = 45 // trace.NewContext [timestamp, internal task id, internal parent task id, stack, name string]
	EventUserTaskEnd       EventType = 46 // end of a task [timestamp, internal task id, stack]
	EventUserRegion        EventType = 47 // trace.WithRegion [timestamp, internal task id, mode(0:start, 1:end), stack, name string]
	EventUserLog           EventType = 48 // trace.Log [timestamp, internal task id, key string id, stack, value string]
	EventCPUSample         EventType = 49 // CPU profiling sample [timestamp, stack, real timestamp, real P id (-1 when absent), goroutine id]
	EventCount             EventType = 50
	// Byte is used but only 6 bits are available for event type.
	// The remaining 2 bits are used to specify the number of arguments.
	// That means, the max event type value is 63.
)
