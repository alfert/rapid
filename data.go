// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package rapid

import (
	"reflect"
	"sync/atomic"
	"time"
)

var seedCounter uint32

type Data interface {
	Draw(g *Generator, label string, unpack ...interface{}) Value
}

type bitStream interface {
	drawBits(n int) uint64
	beginGroup(label string, removable bool) int
	endGroup(i int, discard bool)
}

type bitStreamData struct {
	s bitStream
}

func (d *bitStreamData) Draw(g *Generator, label string, unpack ...interface{}) Value {
	v := g.value(d.s)

	if len(unpack) > 0 {
		unpackTuple(reflect.ValueOf(v), unpack...)
	}

	return v
}

func randomSeed() uint64 {
	return uint64(time.Now().UnixNano()) + uint64(atomic.AddUint32(&seedCounter, 1))
}

type randomBitStream struct {
	jsf64ctx
	recordedBits
}

func newRandomBitStream(seed uint64, persist bool) *randomBitStream {
	s := &randomBitStream{}
	s.init(seed)
	s.persist = persist
	return s
}

func (s *randomBitStream) drawBits(n int) uint64 {
	assert(n >= 0 && n <= 64)

	u := s.rand() & (1<<uint(n) - 1)
	s.record(u)

	return u
}

type bufBitStream struct {
	buf []uint64
	recordedBits
}

func newBufBitStream(buf []uint64, persist bool) *bufBitStream {
	s := &bufBitStream{
		buf: buf,
	}
	s.persist = persist
	return s
}

func (s *bufBitStream) drawBits(n int) uint64 {
	assert(n >= 0 && n <= 64)

	if len(s.buf) == 0 {
		panic(invalidData("overrun"))
	}

	u := s.buf[0] & (1<<uint(n) - 1)
	s.record(u)
	s.buf = s.buf[1:]

	return u
}

type groupInfo struct {
	begin     int
	end       int
	label     string
	removable bool
	discard   bool
}

type recordedBits struct {
	data    []uint64
	groups  []groupInfo
	dataLen int
	persist bool
}

func (rec *recordedBits) record(u uint64) {
	if rec.persist {
		rec.data = append(rec.data, u)
	} else {
		rec.dataLen++
	}
}

func (rec *recordedBits) beginGroup(label string, removable bool) int {
	if !rec.persist {
		return rec.dataLen
	}

	rec.groups = append(rec.groups, groupInfo{
		begin:     len(rec.data),
		end:       -1,
		label:     label,
		removable: removable,
	})

	return len(rec.groups) - 1
}

func (rec *recordedBits) endGroup(i int, discard bool) {
	if !rec.persist {
		assertf(rec.dataLen != i, "group did not use any data from bitstream")
		return
	}

	assertf(len(rec.data) != rec.groups[i].begin, "group did not use any data from bitstream")

	rec.groups[i].end = len(rec.data)
	rec.groups[i].discard = discard
}

func (rec *recordedBits) prune() {
	assert(rec.persist)

	for i := 0; i < len(rec.groups); {
		if rec.groups[i].discard {
			rec.removeGroup(i) // O(n^2)
		} else {
			i++
		}
	}

	for _, g := range rec.groups {
		assert(g.begin != g.end)
	}
}

func (rec *recordedBits) removeGroup(i int) {
	g := rec.groups[i]
	assert(g.end >= 0)

	j := i + 1
	for j < len(rec.groups) && rec.groups[j].end <= g.end {
		j++
	}

	rec.data = append(rec.data[:g.begin], rec.data[g.end:]...)
	rec.groups = append(rec.groups[:i], rec.groups[j:]...)

	n := g.end - g.begin
	for j := 0; j < len(rec.groups); j++ {
		if rec.groups[j].begin >= g.end {
			rec.groups[j].begin -= n
		}
		if rec.groups[j].end >= g.end {
			rec.groups[j].end -= n
		}
	}
}

// "A Small Noncryptographic PRNG" by Bob Jenkins
// See http://www.pcg-random.org/posts/bob-jenkins-small-prng-passes-practrand.html for some recent analysis.
type jsf64ctx struct {
	a uint64
	b uint64
	c uint64
	d uint64
}

func (x *jsf64ctx) init(seed uint64) {
	x.a = 0xf1ea5eed
	x.b = seed
	x.c = seed
	x.d = seed

	for i := 0; i < 20; i++ {
		x.rand()
	}
}

func (x *jsf64ctx) rand() uint64 {
	e := x.a - (x.b<<7 | x.b>>(64-7)) // using bits.RotateLeft64() prevents gc from inlining rand()
	x.a = x.b ^ (x.c<<13 | x.c>>(64-13))
	x.b = x.c + (x.d<<37 | x.d>>(64-37))
	x.c = x.d + e
	x.d = e + x.a
	return x.d
}