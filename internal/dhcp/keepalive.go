package dhcp

import (
	"crypto/md5"
	"encoding/hex"
	"math/rand"

	"go.uber.org/zap"

	"github.com/cyp0633/drcom-go/internal/util"
)

// 第一个保活包
func keepAlive1(salt []byte, authInfo []byte) (err error) {
	// 暂时只能生成没有 keepalive1_mod 的 keepalive1 包
	pkt, err := keepAlive1Nomod(salt, authInfo)
	if err != nil {
		return
	}

	// 发送 keepalive1 包
	_, err = conn.Write(pkt)
	if err == nil {
		err = conn.Flush()
	}
	if err != nil {
		util.Logger.Error("Sending keepalive1 packet failed", zap.Error(err))
		return ErrorKeepalive1
	}
	util.Logger.Debug("Keepalive1 sent", zap.Bool("mod", false), zap.String("packet", hex.EncodeToString(pkt)))

	// 读取keepalive1结果
	result := make([]byte, 1024)
	n, err := conn.Read(result)
	if err != nil {
		util.Logger.Error("Receiving keepalive1 result failed", zap.Error(err))
		return ErrorKeepalive1
	}
	util.Logger.Debug("Keepalive1 recv", zap.String("packet", hex.EncodeToString(result[:n])))
	if result[0] != 0x07 {
		util.Logger.Warn("Bad keepalive1 packet received", zap.String("packet", hex.EncodeToString(result[:n])))
	}
	return
}

// 生成没有 keepalive1_mod 的 keepalive1 包
func keepAlive1Nomod(salt []byte, authInfo []byte) (pkt []byte, err error) {
	pkt = make([]byte, 42)
	// 0xff 0
	pkt[0] = 0xff

	// MD5a 1-16
	md5a := md5.Sum(append(append([]byte{0x03, 0x01}, salt...), util.Conf.Password...))
	copy(pkt[1:], md5a[:])

	// AuthInformation 20-35
	copy(pkt[20:], authInfo)

	// random 36-37
	pkt[36] = byte(rand.Intn(0xFF))
	pkt[37] = byte(rand.Intn(0xFF))
	return
}

var keepAlive2Counter = 0

// 第二个保活包
func keepAlive2(first *bool, encryptType int) error {
	// file packet
	if *first != false {
		pkt := genKeepalive2Packet(*first, 1, 0)
		keepAlive2Counter++
		_, err := conn.Write(pkt)
		if err == nil {
			err = conn.Flush()
		}
		if err != nil {
			util.Logger.Error("Sending keepalive2 packet failed", zap.Error(err))
			return ErrorKeepalive2
		}
		util.Logger.Debug("Keepalive2_file sent", zap.String("packet", hex.EncodeToString(pkt)))

		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			util.Logger.Error("Receiving keepalive2_file result failed", zap.Error(err))
			return ErrorKeepalive2
		}
		util.Logger.Debug("Keepalive2_file recv", zap.String("packet", hex.EncodeToString(buf[:n])))
		if buf[0] == 0x07 {
			if buf[2] == 0x10 {
				util.Logger.Debug("Authentic keepalive2_file recv")
			} else if buf[2] != 0x28 {
				util.Logger.Warn("Bad keepalive2_file packet received", zap.String("packet", hex.EncodeToString(buf[:n])))
				return ErrorKeepalive2
			}
		} else {
			util.Logger.Error("Bad keepalive2_file packet received", zap.String("packet", hex.EncodeToString(buf[:n])))
			return ErrorKeepalive2
		}
	}

	// 心跳包 1 (1/2)
	*first = false
	pkt := genKeepalive2Packet(*first, 1, 0)
	keepAlive2Counter++
	_, err := conn.Write(pkt)
	if err == nil {
		err = conn.Flush()
	}
	if err != nil {
		util.Logger.Error("Sending keepalive2 packet failed", zap.Error(err))
		return ErrorKeepalive2
	}
	util.Logger.Debug("Keepalive2_1 sent", zap.String("packet", hex.EncodeToString(pkt)))

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		util.Logger.Error("Receiving keepalive2_1 result failed", zap.Error(err))
		return ErrorKeepalive2
	}
	util.Logger.Debug("Keepalive2_1 recv", zap.String("packet", hex.EncodeToString(buf[:n])))
	if buf[0] != 0x07 || buf[2] != 0x28 {
		util.Logger.Warn("Bad keepalive2_1 packet received", zap.String("packet", hex.EncodeToString(buf[:n])))
		return ErrorKeepalive2
	}
	tail := buf[16:20]

	// 心跳包 2 (3/4)
	pkt = genKeepalive2Packet(*first, 3, 0)
	for i := 0; i < 4; i++ {
		pkt[16+i] = tail[i]
	}
	_, err = conn.Write(pkt)
	if err == nil {
		err = conn.Flush()
	}
	if err != nil {
		util.Logger.Error("Sending keepalive2_3 packet failed", zap.Error(err))
		return ErrorKeepalive2
	}
	util.Logger.Debug("Keepalive2_3 sent", zap.String("packet", hex.EncodeToString(pkt)))

	buf = make([]byte, 1024)
	n, err = conn.Read(buf)
	if err != nil {
		util.Logger.Error("Receiving keepalive2_3 result failed", zap.Error(err))
		return ErrorKeepalive2
	}
	util.Logger.Debug("Keepalive2_3 recv", zap.String("packet", hex.EncodeToString(buf[:n])))
	if buf[0] != 0x07 || buf[2] != 0x28 {
		util.Logger.Warn("Bad keepalive2_3 packet received", zap.String("packet", hex.EncodeToString(buf[:n])))
		return ErrorKeepalive2
	}
	return nil
}

// 生成第二种保活包
func genKeepalive2Packet(filepacket bool, typ, encryptType int) (pkt []byte) { // 注意 counter 要 &0xff
	pkt = append(pkt, 0x07, byte(keepAlive2Counter&0xff), 0x28, 0x0b, byte(typ))
	if filepacket {
		pkt = append(pkt, 0x0f, 0x27)
	} else {
		pkt = append(pkt, util.Conf.KeepAliveVersion[:]...)
	}
	pkt = append(pkt, 0x2f, 0x12)
	if typ == 3 {
		// [9,27] -> 0x00
		pkt = append(pkt, "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"...)
		// [28,31] -> hostip
		pkt = append(pkt, util.Conf.HostIP...)
	}
	return
}
