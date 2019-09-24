package main

import (
	"github.com/Sirupsen/logrus"
	"net"
	"sync"
	"time"
)

func listen(rule *ruleStructure, wg *sync.WaitGroup) {
	defer wg.Done()
	//监听
	listener, err := net.Listen("tcp", rule.Listen)
	if err != nil {
		logrus.Errorf("[%s] failed to listen at %s", rule.Name, rule.Listen)
		return
	}
	logrus.Infof("[%s] listing at %s", rule.Name, rule.Listen)
	for {
		//处理客户端连接
		conn, err := listener.Accept()
		if err != nil {
			logrus.Errorf("[%s] failed to accept at %s", rule.Name, rule.Listen)
			break
		}
		//判断是否是正则模式
		if rule.EnableRegexp {
			go handleRegexp(conn, rule)
		} else {
			go handleNormal(conn, rule)
		}
	}
	return
}

func handleNormal(conn net.Conn, rule *ruleStructure) {
	var target net.Conn
	//正常模式下挨个连接直到成功连接
	for _, v := range rule.Targets {
		c, err := net.Dial("tcp", v.Address)
		if err != nil {
			logrus.Errorf("[%s] try to handle connection (%s) failed because target (%s) connected failed, try next target.",
				rule.Name, conn.RemoteAddr(), v.Address)
			continue
		}
		target = c
		break
	}
	if target == nil {
		logrus.Errorf("[%s] unable to handle connection (%s) because all targets connected failed",
			rule.Name, conn.RemoteAddr())
		return
	}
	logrus.Debugf("[%s] handle connection (%s) to target (%s)", rule.Name, conn.RemoteAddr(), target.RemoteAddr())

	//io桥
	go tcpBridge(conn, target)
	tcpBridge(target, conn)
}

func handleRegexp(conn net.Conn, rule *ruleStructure) {
	//正则模式下需要客户端的第一个数据包判断特征，所以需要设置一个超时
	conn.SetReadDeadline(time.Now().Add(time.Millisecond * time.Duration(rule.FirstPacketTimeout)))
	//获取第一个数据包
	firstPacket, err := waitFirstPacket(conn)
	if err != nil {
		logrus.Errorf("[%s] unable to handle connection (%s) because failed to get first packet : %s",
			rule.Name, conn.RemoteAddr(), err.Error())
		return
	}

	var target net.Conn
	//挨个匹配正则
	for _, v := range rule.Targets {
		if !v.regexp.Match(firstPacket) {
			continue
		}
		c, err := net.Dial("tcp", v.Address)
		if err != nil {
			logrus.Errorf("[%s] try to handle connection (%s) failed because target (%s) connected failed, try next match target.",
				rule.Name, conn.RemoteAddr(), v.Address)
			continue
		}
		target = c
		break
	}
	if target == nil {
		logrus.Errorf("[%s] unable to handle connection (%s) because no match target",
			rule.Name, conn.RemoteAddr())
		return
	}

	logrus.Debugf("[%s] handle connection (%s) to target (%s)", rule.Name, conn.RemoteAddr(), target.RemoteAddr())
	//匹配到了，去除掉刚才设定的超时
	conn.SetReadDeadline(time.Time{})
	//把第一个数据包发送给目标
	target.Write(firstPacket)

	//io桥
	go tcpBridge(conn, target)
	tcpBridge(target, conn)
}
func waitFirstPacket(conn net.Conn) ([]byte, error) {
	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}
func tcpBridge(a, b net.Conn) {
	defer func() {
		a.Close()
		b.Close()
	}()
	buf := make([]byte, 2048)
	for {
		n, err := a.Read(buf)
		if err != nil {
			return
		}
		b.Write(buf[:n])
	}
}
