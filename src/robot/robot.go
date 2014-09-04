//add  by bingyu

package robot

import (
	//"fmt"
	"bytes"
	"elog"
	"encoding/binary"
	"net"
	//"os"
	//"os/signal"
	"sync"
	"sync/atomic"
	//"syscall"
	"errors"
	"time"
    "encoding/hex"
)

const defRobotNum = 1

//const gateAddr = "192.168.1.10:8001"
const gateAddr = "115.29.139.151:8001"

const defFreq int64 = 60

const defDur = time.Duration(1 * time.Millisecond / 60)

const (
	kRobotActive = iota
	kRobotClose
)

type WaitGroupWrapper struct {
	sync.WaitGroup
}

func (w *WaitGroupWrapper) Run(cb func()) {
	w.Add(1)
	elog.LogDebug("------ wait gropu add 1")
	go func() {
		cb()
		w.Done()
		elog.LogDebug(" wait gropu del  1")
	}()
}

type Message struct {
	id   int32
	Data []byte
}

func ( m *Message ) SetId( id int32 ){
    m.id = id 
}

func ( m*Message ) GetId( ) int32 {
    return m.id 
}

type UPDATE_CALLBACK func(r *Robot)

type Robot struct {
	id        uint32
	addr      *net.TCPAddr
	tcpConn   *net.TCPConn
	recvBuf   *bytes.Buffer
	sendBuf   *bytes.Buffer
	delCh     chan uint32
	closeCh   chan bool
	sendCh    chan Message
	recvCh    chan Message
	state     uint32
	waitGroup *WaitGroupWrapper
	ticker    *time.Ticker
	upCb      UPDATE_CALLBACK
    UserData  interface {}
}

func NewRobot(chDel chan uint32, chClose chan bool, wg *WaitGroupWrapper) *Robot {
	r := &Robot{
		recvBuf:   &bytes.Buffer{},
		sendBuf:   &bytes.Buffer{},
		delCh:     chDel,
		closeCh:   make(chan bool, 1),
		waitGroup: wg,
		state:     kRobotClose,
		sendCh:    make(chan Message, 100),
		recvCh:    make(chan Message, 100),
	}
	return r
}

func (r *Robot) Close() {

	if atomic.CompareAndSwapUint32(&r.state, kRobotActive, kRobotClose) {
		r.delCh <- r.id
        close( r.closeCh )
		elog.LogInfo(" robot:%d close ", r.id)
		r.ticker.Stop()
		r.tcpConn.Close()
	}
}

func  ( r *Robot ) Send( msg Message ){
    r.sendCh <- msg     
    elog.LogInfo(" send  msg : ", msg.id )
}
func (r *Robot) SendLoop() {
	defer r.Close()
	elog.LogDebug(" robot %d  send  loop run  ", r.id)
	for {
		select {
		case msg := <-r.sendCh:
            elog.LogInfo("  encode  msg :%d ", msg.id )
			binary.Write(r.sendBuf, binary.LittleEndian, msg.id)
			binary.Write(r.sendBuf, binary.LittleEndian, msg.Data)
			byte := r.sendBuf.Bytes()
			n, err := r.tcpConn.Write(byte)
			if err != nil {
				elog.LogSysln(" conn ", r.id, " write data fail :", err)
				return
			}

            elog.LogInfo(" write  msg :%d ", n, hex.Dump( r.sendBuf.Bytes()) )
		case <-r.closeCh:
			elog.LogDebug(" send loop begin close ")
			return
		}
	}

	elog.LogDebug("send loop close  ")
}

func (r *Robot) ReadLoop() {
	defer r.Close()
	elog.LogDebug(" robot %d  read loop run  ", r.id)
	buf := make([]byte, 1024*1024)
	for {
		select {
		case <-r.closeCh:
			elog.LogSysln("read loop begin stop ")
			return
		default:
		}

		r.tcpConn.SetDeadline(time.Now().Add(1e9))
		n, err := r.tcpConn.Read(buf)
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
				continue
			}
			elog.LogErrorln(" read data error ", err)
			return
		}
		elog.LogSys(" ********* read data : %d ", n )
        r.recvBuf.Write(buf[0:n])

		var msg Message
		binary.Read(r.recvBuf, binary.LittleEndian, msg.id)
		msg.Data = make([]byte, r.recvBuf.Len())
		r.recvBuf.Read(msg.Data)

		r.recvCh <- msg
	}

}

func (r *Robot) Run() {

	elog.LogDebug(" robot %d begin run ", r.id)
	var err error
	r.addr, err = net.ResolveTCPAddr("tcp", gateAddr)
	if err != nil {
		elog.LogErrorln(gateAddr, ":resolve tcp addr fail, please usage: 0.0.0.1:2345, fail: ", err)
		return
	}
	r.tcpConn, err = net.DialTCP("tcp", nil, r.addr)
	if err != nil {
		elog.LogErrorln("connect server fail , because :", err)
		return
	}

	elog.LogInfoln(" connect  server sucess :", r.id, r.tcpConn.RemoteAddr().String())
	r.state = kRobotActive

	r.waitGroup.Run(r.SendLoop)
	r.waitGroup.Run(r.ReadLoop)

	r.ticker = time.NewTicker(defDur)

	defer r.Close()
	for {
		select {
		case <-r.closeCh:
			elog.LogDebugln("update loop begin stop ")
			return
		case <-r.ticker.C:
			//逻辑处理
			r.update()
			//elog.LogInfoln("robot :", r.id, "heart :", t)
		default:
		}
	}
}

func (r *Robot) GetMsg() (msg Message, err error) {
	select {
	    case msg = <-r.recvCh:
		    elog.LogInfo("receieve msg :%s ", msg.id)
		    err = nil
		    return
        default :
	}
	err = errors.New(" not msg ")
	return
}

func (r *Robot) SetUpdateCb(cb UPDATE_CALLBACK) {
	r.upCb = cb
}

func (r *Robot) GetId() uint32 {
	return r.id
}
func (r *Robot) update() {
	if r.upCb != nil {
		r.upCb(r)
	}
}

type RobotMng struct {
	lastRobotId uint32
	robots      map[uint32]*Robot
	mapMutex    sync.Mutex
	delCh       chan uint32
	ticker      *time.Ticker
	waitGroup   *WaitGroupWrapper
	closeCh     chan bool
	upCb        UPDATE_CALLBACK
}

func (rbMng *RobotMng) SetUpdateCb(cb UPDATE_CALLBACK) {
	rbMng.upCb = cb
}

func (rbMng *RobotMng) AddRobot(r *Robot) {
	rbMng.mapMutex.Lock()
	defer rbMng.mapMutex.Unlock()
	rbMng.robots[r.id] = r
	elog.LogDebugln("add robot ", r.id)
}

func (rbMng *RobotMng) DelRobot(id uint32) {
	rbMng.mapMutex.Lock()
	defer rbMng.mapMutex.Unlock()
	delete(rbMng.robots, id)
	elog.LogDebugln("del robot ", id)
}

func (rbMng *RobotMng) NewRobot() {

	r := NewRobot(rbMng.delCh, rbMng.closeCh, rbMng.waitGroup)
	r.id = atomic.AddUint32(&rbMng.lastRobotId, 1)
	rbMng.AddRobot(r)
	elog.LogDebug(" create robot %d ", r.id)
	r.SetUpdateCb(rbMng.upCb)
	rbMng.waitGroup.Run(r.Run)
}

func (rbMng *RobotMng) Close() {

	elog.LogDebug(" .......................begin close ........................")
	rbMng.ticker.Stop()
	elog.LogDebug(" rbMng ticket stop")
    for _, r := range rbMng.robots {
        r.Close()
    }
	close(rbMng.closeCh)
    elog.LogDebug(" rbmng  closech already close ")
	rbMng.waitGroup.Wait()
	elog.LogDebug(" rbMng  wait all robot close ")
	//最后关del ch
	close(rbMng.delCh)
	elog.LogDebug(" rbMng delCh close ")
	elog.LogDebug(" Everything is ok, i quit ....................................")
}

func (rbMng *RobotMng) Heart() {

	//500 ms 检测一次
	rbMng.ticker = time.NewTicker(defDur)
	for {
		// del close
		select {
		case id := <-rbMng.delCh:
			rbMng.DelRobot(id)
		case <-rbMng.closeCh:
			elog.LogDebugln(" rbMng heart close ")
			return
		case <-rbMng.ticker.C:
			//定时检测
			diff := defRobotNum - len(rbMng.robots)
			if diff > 0 {
				elog.LogDebugln(" RobotMng heart : ", "diff ", diff)
				for i := 0; i < diff; i++ {
					rbMng.NewRobot()
				}
			}
		default:
		}
	}
}

func (rbMng *RobotMng) Run() {
	go rbMng.waitGroup.Run(rbMng.Heart)
}

func NewRobotMng() *RobotMng {
	robotMng := &RobotMng{
		robots:      make(map[uint32]*Robot),
		lastRobotId: 0,
		delCh:       make(chan uint32, 100),
		closeCh:     make(chan bool),
		waitGroup:   &WaitGroupWrapper{},
	}

	return robotMng
}
