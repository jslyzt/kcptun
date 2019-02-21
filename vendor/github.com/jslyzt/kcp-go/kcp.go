// Package kcp - A Fast and Reliable ARQ Protocol
package kcp

import (
	"encoding/binary"
	"sync/atomic"
)

// const define
const (
	IkcpRtoNdl     = 30  // no delay min rto
	IkcpRtoMin     = 100 // normal min rto
	IkcpRtoDef     = 200
	IkcpRtoMax     = 60000
	IkcpCmdPush    = 81 // cmd: push data
	IkcpCmdAck     = 82 // cmd: ack
	IkcpCmdWask    = 83 // cmd: window probe (ask)
	IkcpCmdWins    = 84 // cmd: window size (tell)
	IkcpAskSend    = 1  // need to send IkcpCmdWask
	IkcpAskTell    = 2  // need to send IkcpCmdWins
	IkcpWndSnd     = 32
	IkcpWndRcv     = 32
	IkcpMtuDef     = 1400
	IkcpAckFast    = 3
	IkcpInterval   = 100
	IkcpOverhead   = 28
	IkcpDeadlink   = 20
	IkcpThreshInit = 2
	IkcpThreshMin  = 2
	IkcpProbeInit  = 7000   // 7 secs to probe window size
	IkcpProbeLimit = 120000 // up to 120 secs to probe window
)

// outputCallback is a prototype which ought capture conn and call conn.Write
type outputCallback func(buf []byte, size int)

/* encode 8 bits unsigned int */
func ikcpEncode8u(p []byte, c byte) []byte {
	p[0] = c
	return p[1:]
}

/* decode 8 bits unsigned int */
func ikcpDecode8u(p []byte, c *byte) []byte {
	*c = p[0]
	return p[1:]
}

/* encode 16 bits unsigned int (lsb) */
func ikcpEncode16u(p []byte, w uint16) []byte {
	binary.LittleEndian.PutUint16(p, w)
	return p[2:]
}

/* decode 16 bits unsigned int (lsb) */
func ikcpDecode16u(p []byte, w *uint16) []byte {
	*w = binary.LittleEndian.Uint16(p)
	return p[2:]
}

/* encode 32 bits unsigned int (lsb) */
func ikcpEncode32u(p []byte, l uint32) []byte {
	binary.LittleEndian.PutUint32(p, l)
	return p[4:]
}

/* decode 32 bits unsigned int (lsb) */
func ikcpDecode32u(p []byte, l *uint32) []byte {
	*l = binary.LittleEndian.Uint32(p)
	return p[4:]
}

/* encode 64 bits unsigned int (lsb) */
func ikcpEncode64u(p []byte, l uint64) []byte {
	binary.LittleEndian.PutUint64(p, l)
	return p[8:]
}

/* decode 64 bits unsigned int (lsb) */
func ikcpDecode64u(p []byte, l *uint64) []byte {
	*l = binary.LittleEndian.Uint64(p)
	return p[8:]
}

func _imin(a, b uint32) uint32 {
	if a <= b {
		return a
	}
	return b
}

func _imax(a, b uint32) uint32 {
	if a >= b {
		return a
	}
	return b
}

func _ibound(lower, middle, upper uint32) uint32 {
	return _imin(_imax(lower, middle), upper)
}

func _itimediff(later, earlier uint32) int32 {
	return (int32)(later - earlier)
}

// Segment defines a KCP segment
type Segment struct {
	conv     uint64
	cmd      uint8
	frg      uint8
	wnd      uint16
	ts       uint32
	sn       uint32
	una      uint32
	rto      uint32
	xmit     uint32
	resendts uint32
	fastack  uint32
	acked    uint32 // mark if the seg has acked
	data     []byte
}

// encode a segment into buffer
func (seg *Segment) encode(ptr []byte) []byte {
	ptr = ikcpEncode64u(ptr, seg.conv)
	ptr = ikcpEncode8u(ptr, seg.cmd)
	ptr = ikcpEncode8u(ptr, seg.frg)
	ptr = ikcpEncode16u(ptr, seg.wnd)
	ptr = ikcpEncode32u(ptr, seg.ts)
	ptr = ikcpEncode32u(ptr, seg.sn)
	ptr = ikcpEncode32u(ptr, seg.una)
	ptr = ikcpEncode32u(ptr, uint32(len(seg.data)))
	atomic.AddUint64(&DefaultSnmp.OutSegs, 1)
	return ptr
}

// KCP defines a single KCP connection
type KCP struct {
	conv                                uint64
	mtu, mss, state                     uint32
	sndUna, sndNxt, rcvNxt              uint32
	ssthresh                            uint32
	rxRttvar, rxSrtt                    int32
	rxRto, rxMinrto                     uint32
	sndWnd, rcvWnd, rmtWnd, cwnd, probe uint32
	interval, tsFlush                   uint32
	nodelay, updated                    uint32
	tsProbe, probeWait                  uint32
	deadLink, incr                      uint32

	fastresend     int32
	nocwnd, stream int32

	sndQueue []*Segment
	rcvQueue []*Segment
	sndBuf   []*Segment
	rcvBuf   []*Segment

	acklist []ackItem

	buffer []byte
	output outputCallback
}

type ackItem struct {
	sn uint32
	ts uint32
}

// NewKCP create a new kcp control object, 'conv' must equal in two endpoint
// from the same connection.
func NewKCP(conv uint64, output outputCallback) *KCP {
	kcp := new(KCP)
	kcp.conv = conv
	kcp.sndWnd = IkcpWndSnd
	kcp.rcvWnd = IkcpWndRcv
	kcp.rmtWnd = IkcpWndRcv
	kcp.mtu = IkcpMtuDef
	kcp.mss = kcp.mtu - IkcpOverhead
	kcp.buffer = make([]byte, (kcp.mtu+IkcpOverhead)*3)
	kcp.rxRto = IkcpRtoDef
	kcp.rxMinrto = IkcpRtoMin
	kcp.interval = IkcpInterval
	kcp.tsFlush = IkcpInterval
	kcp.ssthresh = IkcpThreshInit
	kcp.deadLink = IkcpDeadlink
	kcp.output = output
	return kcp
}

// newSegment creates a KCP segment
func (kcp *KCP) newSegment(size int) *Segment {
	return &Segment{
		data: xmitBuf.Get().([]byte)[:size],
	}
}

// delSegment recycles a KCP segment
func (kcp *KCP) delSegment(seg *Segment) {
	if seg.data != nil {
		xmitBuf.Put(seg.data)
		seg.data = nil
	}
}

// PeekSize checks the size of next message in the recv queue
func (kcp *KCP) PeekSize() (length int) {
	if len(kcp.rcvQueue) == 0 {
		return -1
	}

	seg := kcp.rcvQueue[0]
	if seg.frg == 0 {
		return len(seg.data)
	}

	if len(kcp.rcvQueue) < int(seg.frg+1) {
		return -1
	}

	for _, sseg := range kcp.rcvQueue {
		if sseg == nil {
			continue
		}
		length += len(sseg.data)
		if sseg.frg == 0 {
			break
		}
	}
	return
}

// Recv is user/upper level recv: returns size, returns below zero for EAGAIN
func (kcp *KCP) Recv(buffer []byte) (n int) {
	if len(kcp.rcvQueue) == 0 {
		return -1
	}

	peeksize := kcp.PeekSize()
	if peeksize < 0 {
		return -2
	}

	if peeksize > len(buffer) {
		return -3
	}

	var fastRecover bool
	if len(kcp.rcvQueue) >= int(kcp.rcvWnd) {
		fastRecover = true
	}

	// merge fragment
	count := 0
	for _, seg := range kcp.rcvQueue {
		if seg == nil {
			continue
		}
		copy(buffer, seg.data)
		buffer = buffer[len(seg.data):]
		n += len(seg.data)
		count++
		kcp.delSegment(seg)
		if seg.frg == 0 {
			break
		}
	}
	if count > 0 {
		kcp.rcvQueue = kcp.removeFront(kcp.rcvQueue, count)
	}

	// move available data from rcvBuf -> rcvQueue
	count = 0
	for _, seg := range kcp.rcvBuf {
		if seg == nil {
			continue
		}
		if seg.sn == kcp.rcvNxt && len(kcp.rcvQueue) < int(kcp.rcvWnd) {
			kcp.rcvNxt++
			count++
		} else {
			break
		}
	}

	if count > 0 {
		kcp.rcvQueue = append(kcp.rcvQueue, kcp.rcvBuf[:count]...)
		kcp.rcvBuf = kcp.removeFront(kcp.rcvBuf, count)
	}

	// fast recover
	if len(kcp.rcvQueue) < int(kcp.rcvWnd) && fastRecover {
		// ready to send back IkcpCmdWins in ikcp_flush
		// tell remote my window size
		kcp.probe |= IkcpAskTell
	}
	return
}

// Send is user/upper level send, returns below zero for error
func (kcp *KCP) Send(buffer []byte) int {
	var count int
	if len(buffer) == 0 {
		return -1
	}

	// append to previous segment in streaming mode (if possible)
	if kcp.stream != 0 {
		n := len(kcp.sndQueue)
		if n > 0 {
			seg := kcp.sndQueue[n-1]
			if len(seg.data) < int(kcp.mss) {
				capacity := int(kcp.mss) - len(seg.data)
				extend := capacity
				if len(buffer) < capacity {
					extend = len(buffer)
				}

				// grow slice, the underlying cap is guaranteed to
				// be larger than kcp.mss
				oldlen := len(seg.data)
				seg.data = seg.data[:oldlen+extend]
				copy(seg.data[oldlen:], buffer)
				buffer = buffer[extend:]
			}
		}

		if len(buffer) == 0 {
			return 0
		}
	}

	if len(buffer) <= int(kcp.mss) {
		count = 1
	} else {
		count = (len(buffer) + int(kcp.mss) - 1) / int(kcp.mss)
	}

	if count > 255 {
		return -2
	}

	if count == 0 {
		count = 1
	}

	for i := 0; i < count; i++ {
		var size int
		if len(buffer) > int(kcp.mss) {
			size = int(kcp.mss)
		} else {
			size = len(buffer)
		}
		seg := kcp.newSegment(size)
		copy(seg.data, buffer[:size])
		if kcp.stream == 0 { // message mode
			seg.frg = uint8(count - i - 1)
		} else { // stream mode
			seg.frg = 0
		}
		kcp.sndQueue = append(kcp.sndQueue, seg)
		buffer = buffer[size:]
	}
	return 0
}

func (kcp *KCP) updateAck(rtt int32) {
	// https://tools.ietf.org/html/rfc6298
	var rto uint32
	if kcp.rxSrtt == 0 {
		kcp.rxSrtt = rtt
		kcp.rxRttvar = rtt >> 1
	} else {
		delta := rtt - kcp.rxSrtt
		kcp.rxSrtt += delta >> 3
		if delta < 0 {
			delta = -delta
		}
		if rtt < kcp.rxSrtt-kcp.rxRttvar {
			// if the new RTT sample is below the bottom of the range of
			// what an RTT measurement is expected to be.
			// give an 8x reduced weight versus its normal weighting
			kcp.rxRttvar += (delta - kcp.rxRttvar) >> 5
		} else {
			kcp.rxRttvar += (delta - kcp.rxRttvar) >> 2
		}
	}
	rto = uint32(kcp.rxSrtt) + _imax(kcp.interval, uint32(kcp.rxRttvar)<<2)
	kcp.rxRto = _ibound(kcp.rxMinrto, rto, IkcpRtoMax)
}

func (kcp *KCP) shrinkBuf() {
	if len(kcp.sndBuf) > 0 {
		seg := kcp.sndBuf[0]
		kcp.sndUna = seg.sn
	} else {
		kcp.sndUna = kcp.sndNxt
	}
}

func (kcp *KCP) parseAck(sn uint32) {
	if _itimediff(sn, kcp.sndUna) < 0 || _itimediff(sn, kcp.sndNxt) >= 0 {
		return
	}

	for _, seg := range kcp.sndBuf {
		if seg == nil {
			continue
		}
		if sn == seg.sn {
			seg.acked = 1
			kcp.delSegment(seg)
			break
		}
		if _itimediff(sn, seg.sn) < 0 {
			break
		}
	}
}

func (kcp *KCP) parseFastack(sn, ts uint32) {
	if _itimediff(sn, kcp.sndUna) < 0 || _itimediff(sn, kcp.sndNxt) >= 0 {
		return
	}

	for _, seg := range kcp.sndBuf {
		if seg == nil {
			continue
		}
		if _itimediff(sn, seg.sn) < 0 {
			break
		} else if sn != seg.sn && _itimediff(seg.ts, ts) <= 0 {
			seg.fastack++
		}
	}
}

func (kcp *KCP) parseUna(una uint32) {
	count := 0
	for _, seg := range kcp.sndBuf {
		if seg == nil {
			continue
		}
		if _itimediff(una, seg.sn) > 0 {
			kcp.delSegment(seg)
			count++
		} else {
			break
		}
	}
	if count > 0 {
		kcp.sndBuf = kcp.removeFront(kcp.sndBuf, count)
	}
}

// ack append
func (kcp *KCP) ackPush(sn, ts uint32) {
	kcp.acklist = append(kcp.acklist, ackItem{sn, ts})
}

// returns true if data has repeated
func (kcp *KCP) parseData(newseg *Segment) bool {
	sn := newseg.sn
	if _itimediff(sn, kcp.rcvNxt+kcp.rcvWnd) >= 0 ||
		_itimediff(sn, kcp.rcvNxt) < 0 {
		return true
	}

	n := len(kcp.rcvBuf) - 1
	insertIdx := 0
	repeat := false
	for i := n; i >= 0; i-- {
		seg := kcp.rcvBuf[i]
		if seg.sn == sn {
			repeat = true
			break
		}
		if _itimediff(sn, seg.sn) > 0 {
			insertIdx = i + 1
			break
		}
	}

	if !repeat {
		// replicate the content if it's new
		dataCopy := xmitBuf.Get().([]byte)[:len(newseg.data)]
		copy(dataCopy, newseg.data)
		newseg.data = dataCopy

		if insertIdx == n+1 {
			kcp.rcvBuf = append(kcp.rcvBuf, newseg)
		} else {
			kcp.rcvBuf = append(kcp.rcvBuf, &Segment{})
			copy(kcp.rcvBuf[insertIdx+1:], kcp.rcvBuf[insertIdx:])
			kcp.rcvBuf[insertIdx] = newseg
		}
	}

	// move available data from rcvBuf -> rcvQueue
	count := 0
	for _, seg := range kcp.rcvBuf {
		if seg == nil {
			continue
		}
		if seg.sn == kcp.rcvNxt && len(kcp.rcvQueue) < int(kcp.rcvWnd) {
			kcp.rcvNxt++
			count++
		} else {
			break
		}
	}
	if count > 0 {
		kcp.rcvQueue = append(kcp.rcvQueue, kcp.rcvBuf[:count]...)
		kcp.rcvBuf = kcp.removeFront(kcp.rcvBuf, count)
	}

	return repeat
}

// Input when you received a low level packet (eg. UDP packet), call it
// regular indicates a regular packet has received(not from FEC)
func (kcp *KCP) Input(data []byte, regular, ackNoDelay bool) int {
	sndUna := kcp.sndUna
	if len(data) < IkcpOverhead {
		return -1
	}

	var (
		latest uint32 // latest packet
		flag   int
		inSegs uint64
		conv   uint64

		ts, sn, length, una uint32
		wnd                 uint16
		cmd, frg            uint8
	)

	for {
		if len(data) < int(IkcpOverhead) {
			break
		}

		data = ikcpDecode64u(data, &conv)
		if conv != kcp.conv {
			return -1
		}

		data = ikcpDecode8u(data, &cmd)
		data = ikcpDecode8u(data, &frg)
		data = ikcpDecode16u(data, &wnd)
		data = ikcpDecode32u(data, &ts)
		data = ikcpDecode32u(data, &sn)
		data = ikcpDecode32u(data, &una)
		data = ikcpDecode32u(data, &length)
		if len(data) < int(length) {
			return -2
		}

		if cmd != IkcpCmdPush && cmd != IkcpCmdAck &&
			cmd != IkcpCmdWask && cmd != IkcpCmdWins {
			return -3
		}

		// only trust window updates from regular packets. i.e: latest update
		if regular {
			kcp.rmtWnd = uint32(wnd)
		}
		kcp.parseUna(una)
		kcp.shrinkBuf()

		if cmd == IkcpCmdAck {
			kcp.parseAck(sn)
			// stricter check of fastack
			kcp.parseFastack(sn, ts)
			if flag == 0 {
				flag = 1
				latest = ts
			} else if _itimediff(ts, latest) > 0 {
				latest = ts
			}
		} else if cmd == IkcpCmdPush {
			repeat := true
			if _itimediff(sn, kcp.rcvNxt+kcp.rcvWnd) < 0 {
				kcp.ackPush(sn, ts)
				if _itimediff(sn, kcp.rcvNxt) >= 0 {
					repeat = kcp.parseData(&Segment{
						conv: conv,
						cmd:  cmd,
						frg:  frg,
						wnd:  wnd,
						ts:   ts,
						sn:   sn,
						una:  una,
						data: data[:length], // delayed data copying
					})
				}
			}
			if regular && repeat {
				atomic.AddUint64(&DefaultSnmp.RepeatSegs, 1)
			}
		} else if cmd == IkcpCmdWask {
			// ready to send back IkcpCmdWins in Ikcp_flush
			// tell remote my window size
			kcp.probe |= IkcpAskTell
		} else if cmd == IkcpCmdWins {
			// do nothing
		} else {
			return -3
		}

		inSegs++
		data = data[length:]
	}
	atomic.AddUint64(&DefaultSnmp.InSegs, inSegs)

	// update rtt with the latest ts
	// ignore the FEC packet
	if flag != 0 && regular {
		current := currentMs()
		if _itimediff(current, latest) >= 0 {
			kcp.updateAck(_itimediff(current, latest))
		}
	}

	if _itimediff(kcp.sndUna, sndUna) > 0 {
		if kcp.cwnd < kcp.rmtWnd {
			mss := kcp.mss
			if kcp.cwnd < kcp.ssthresh {
				kcp.cwnd++
				kcp.incr += mss
			} else {
				if kcp.incr < mss {
					kcp.incr = mss
				}
				kcp.incr += (mss*mss)/kcp.incr + (mss / 16)
				if (kcp.cwnd+1)*mss <= kcp.incr {
					kcp.cwnd++
				}
			}
			if kcp.cwnd > kcp.rmtWnd {
				kcp.cwnd = kcp.rmtWnd
				kcp.incr = kcp.rmtWnd * mss
			}
		}
	}

	if ackNoDelay && len(kcp.acklist) > 0 { // ack immediately
		kcp.flush(true)
	}
	return 0
}

func (kcp *KCP) wndUnused() uint16 {
	if len(kcp.rcvQueue) < int(kcp.rcvWnd) {
		return uint16(int(kcp.rcvWnd) - len(kcp.rcvQueue))
	}
	return 0
}

// flush pending data
func (kcp *KCP) flush(ackOnly bool) uint32 {
	seg := &Segment{
		conv: kcp.conv,
		cmd:  IkcpCmdAck,
		wnd:  kcp.wndUnused(),
		una:  kcp.rcvNxt,
	}

	buffer := kcp.buffer
	// flush acknowledges
	ptr := buffer
	for i, ack := range kcp.acklist {
		size := len(buffer) - len(ptr)
		if size+IkcpOverhead > int(kcp.mtu) {
			kcp.output(buffer, size)
			ptr = buffer
		}
		// filter jitters caused by bufferbloat
		if ack.sn >= kcp.rcvNxt || len(kcp.acklist)-1 == i {
			seg.sn, seg.ts = ack.sn, ack.ts
			ptr = seg.encode(ptr)
		}
	}
	kcp.acklist = kcp.acklist[0:0]

	if ackOnly { // flash remain ack segments
		size := len(buffer) - len(ptr)
		if size > 0 {
			kcp.output(buffer, size)
		}
		return kcp.interval
	}

	// probe window size (if remote window size equals zero)
	if kcp.rmtWnd == 0 {
		current := currentMs()
		if kcp.probeWait == 0 {
			kcp.probeWait = IkcpProbeInit
			kcp.tsProbe = current + kcp.probeWait
		} else {
			if _itimediff(current, kcp.tsProbe) >= 0 {
				if kcp.probeWait < IkcpProbeInit {
					kcp.probeWait = IkcpProbeInit
				}
				kcp.probeWait += kcp.probeWait / 2
				if kcp.probeWait > IkcpProbeLimit {
					kcp.probeWait = IkcpProbeLimit
				}
				kcp.tsProbe = current + kcp.probeWait
				kcp.probe |= IkcpAskSend
			}
		}
	} else {
		kcp.tsProbe = 0
		kcp.probeWait = 0
	}

	// flush window probing commands
	if (kcp.probe & IkcpAskSend) != 0 {
		seg.cmd = IkcpCmdWask
		size := len(buffer) - len(ptr)
		if size+IkcpOverhead > int(kcp.mtu) {
			kcp.output(buffer, size)
			ptr = buffer
		}
		ptr = seg.encode(ptr)
	}

	// flush window probing commands
	if (kcp.probe & IkcpAskTell) != 0 {
		seg.cmd = IkcpCmdWins
		size := len(buffer) - len(ptr)
		if size+IkcpOverhead > int(kcp.mtu) {
			kcp.output(buffer, size)
			ptr = buffer
		}
		ptr = seg.encode(ptr)
	}

	kcp.probe = 0

	// calculate window size
	cwnd := _imin(kcp.sndWnd, kcp.rmtWnd)
	if kcp.nocwnd == 0 {
		cwnd = _imin(kcp.cwnd, cwnd)
	}

	// sliding window, controlled by sndNxt && sna_una+cwnd
	newSegsCount := 0
	for k := range kcp.sndQueue {
		if _itimediff(kcp.sndNxt, kcp.sndUna+cwnd) >= 0 {
			break
		}
		newseg := kcp.sndQueue[k]
		newseg.conv = kcp.conv
		newseg.cmd = IkcpCmdPush
		newseg.sn = kcp.sndNxt
		kcp.sndBuf = append(kcp.sndBuf, newseg)
		kcp.sndNxt++
		newSegsCount++
	}
	if newSegsCount > 0 {
		kcp.sndQueue = kcp.removeFront(kcp.sndQueue, newSegsCount)
	}

	// calculate resent
	resent := uint32(kcp.fastresend)
	if kcp.fastresend <= 0 {
		resent = 0xffffffff
	}

	// check for retransmissions
	current := currentMs()
	var change, lost, lostSegs, fastRetransSegs, earlyRetransSegs uint64
	minrto := int32(kcp.interval)

	ref := kcp.sndBuf[:len(kcp.sndBuf)] // for bounds check elimination
	for _, segment := range ref {
		if segment == nil {
			continue
		}
		needsend := false
		if segment.acked == 1 {
			continue
		}
		if segment.xmit == 0 { // initial transmit
			needsend = true
			segment.rto = kcp.rxRto
			segment.resendts = current + segment.rto
		} else if _itimediff(current, segment.resendts) >= 0 { // RTO
			needsend = true
			if kcp.nodelay == 0 {
				segment.rto += kcp.rxRto
			} else {
				segment.rto += kcp.rxRto / 2
			}
			segment.resendts = current + segment.rto
			lost++
			lostSegs++
		} else if segment.fastack >= resent { // fast retransmit
			needsend = true
			segment.fastack = 0
			segment.rto = kcp.rxRto
			segment.resendts = current + segment.rto
			change++
			fastRetransSegs++
		} else if segment.fastack > 0 && newSegsCount == 0 { // early retransmit
			needsend = true
			segment.fastack = 0
			segment.rto = kcp.rxRto
			segment.resendts = current + segment.rto
			change++
			earlyRetransSegs++
		}

		if needsend {
			current = currentMs() // time update for a blocking call
			segment.xmit++
			segment.ts = current
			segment.wnd = seg.wnd
			segment.una = seg.una

			size := len(buffer) - len(ptr)
			need := IkcpOverhead + len(segment.data)

			if size+need > int(kcp.mtu) {
				kcp.output(buffer, size)
				ptr = buffer
			}

			ptr = segment.encode(ptr)
			copy(ptr, segment.data)
			ptr = ptr[len(segment.data):]

			if segment.xmit >= kcp.deadLink {
				kcp.state = 0xFFFFFFFF
			}
		}

		// get the nearest rto
		if rto := _itimediff(segment.resendts, current); rto > 0 && rto < minrto {
			minrto = rto
		}
	}

	// flash remain segments
	size := len(buffer) - len(ptr)
	if size > 0 {
		kcp.output(buffer, size)
	}

	// counter updates
	sum := lostSegs
	if lostSegs > 0 {
		atomic.AddUint64(&DefaultSnmp.LostSegs, lostSegs)
	}
	if fastRetransSegs > 0 {
		atomic.AddUint64(&DefaultSnmp.FastRetransSegs, fastRetransSegs)
		sum += fastRetransSegs
	}
	if earlyRetransSegs > 0 {
		atomic.AddUint64(&DefaultSnmp.EarlyRetransSegs, earlyRetransSegs)
		sum += earlyRetransSegs
	}
	if sum > 0 {
		atomic.AddUint64(&DefaultSnmp.RetransSegs, sum)
	}

	// update ssthresh
	// rate halving, https://tools.ietf.org/html/rfc6937
	if change > 0 {
		inflight := kcp.sndNxt - kcp.sndUna
		kcp.ssthresh = inflight / 2
		if kcp.ssthresh < IkcpThreshMin {
			kcp.ssthresh = IkcpThreshMin
		}
		kcp.cwnd = kcp.ssthresh + resent
		kcp.incr = kcp.cwnd * kcp.mss
	}

	// congestion control, https://tools.ietf.org/html/rfc5681
	if lost > 0 {
		kcp.ssthresh = cwnd / 2
		if kcp.ssthresh < IkcpThreshMin {
			kcp.ssthresh = IkcpThreshMin
		}
		kcp.cwnd = 1
		kcp.incr = kcp.mss
	}

	if kcp.cwnd < 1 {
		kcp.cwnd = 1
		kcp.incr = kcp.mss
	}

	return uint32(minrto)
}

// Update updates state (call it repeatedly, every 10ms-100ms), or you can ask
// ikcp_check when to call it again (without ikcp_input/_send calling).
// 'current' - current timestamp in millisec.
func (kcp *KCP) Update() {
	var slap int32

	current := currentMs()
	if kcp.updated == 0 {
		kcp.updated = 1
		kcp.tsFlush = current
	}

	slap = _itimediff(current, kcp.tsFlush)

	if slap >= 10000 || slap < -10000 {
		kcp.tsFlush = current
		slap = 0
	}

	if slap >= 0 {
		kcp.tsFlush += kcp.interval
		if _itimediff(current, kcp.tsFlush) >= 0 {
			kcp.tsFlush = current + kcp.interval
		}
		kcp.flush(false)
	}
}

// Check determines when should you invoke ikcp_update:
// returns when you should invoke ikcp_update in millisec, if there
// is no ikcp_input/_send calling. you can call ikcp_update in that
// time, instead of call update repeatly.
// Important to reduce unnacessary ikcp_update invoking. use it to
// schedule ikcp_update (eg. implementing an epoll-like mechanism,
// or optimize ikcp_update when handling massive kcp connections)
func (kcp *KCP) Check() uint32 {
	current := currentMs()
	tsFlush := kcp.tsFlush
	tmFlush := int32(0x7fffffff)
	tmPacket := int32(0x7fffffff)
	minimal := uint32(0)
	if kcp.updated == 0 {
		return current
	}

	if _itimediff(current, tsFlush) >= 10000 ||
		_itimediff(current, tsFlush) < -10000 {
		tsFlush = current
	}

	if _itimediff(current, tsFlush) >= 0 {
		return current
	}

	tmFlush = _itimediff(tsFlush, current)

	for _, seg := range kcp.sndBuf {
		if seg == nil {
			continue
		}
		diff := _itimediff(seg.resendts, current)
		if diff <= 0 {
			return current
		}
		if diff < tmPacket {
			tmPacket = diff
		}
	}

	minimal = uint32(tmPacket)
	if tmPacket >= tmFlush {
		minimal = uint32(tmFlush)
	}
	if minimal >= kcp.interval {
		minimal = kcp.interval
	}

	return current + minimal
}

// SetMtu changes MTU size, default is 1400
func (kcp *KCP) SetMtu(mtu int) int {
	if mtu < 50 || mtu < IkcpOverhead {
		return -1
	}
	buffer := make([]byte, (mtu+IkcpOverhead)*3)
	if buffer == nil {
		return -2
	}
	kcp.mtu = uint32(mtu)
	kcp.mss = kcp.mtu - IkcpOverhead
	kcp.buffer = buffer
	return 0
}

// NoDelay options
// fastest: ikcp_nodelay(kcp, 1, 20, 2, 1)
// nodelay: 0:disable(default), 1:enable
// interval: internal update timer interval in millisec, default is 100ms
// resend: 0:disable fast resend(default), 1:enable fast resend
// nc: 0:normal congestion control(default), 1:disable congestion control
func (kcp *KCP) NoDelay(nodelay, interval, resend, nc int) int {
	if nodelay >= 0 {
		kcp.nodelay = uint32(nodelay)
		if nodelay != 0 {
			kcp.rxMinrto = IkcpRtoNdl
		} else {
			kcp.rxMinrto = IkcpRtoMin
		}
	}
	if interval >= 0 {
		if interval > 5000 {
			interval = 5000
		} else if interval < 10 {
			interval = 10
		}
		kcp.interval = uint32(interval)
	}
	if resend >= 0 {
		kcp.fastresend = int32(resend)
	}
	if nc >= 0 {
		kcp.nocwnd = int32(nc)
	}
	return 0
}

// WndSize sets maximum window size: sndwnd=32, rcvwnd=32 by default
func (kcp *KCP) WndSize(sndwnd, rcvwnd int) int {
	if sndwnd > 0 {
		kcp.sndWnd = uint32(sndwnd)
	}
	if rcvwnd > 0 {
		kcp.rcvWnd = uint32(rcvwnd)
	}
	return 0
}

// WaitSnd gets how many packet is waiting to be sent
func (kcp *KCP) WaitSnd() int {
	return len(kcp.sndBuf) + len(kcp.sndQueue)
}

// remove front n elements from queue
func (kcp *KCP) removeFront(q []*Segment, n int) []*Segment {
	newn := copy(q, q[n:])
	return q[:newn]
}
