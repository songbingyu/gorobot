package main

import (
	"elog"
	"os"
	"os/signal"
	"robot"
    "strconv"
	"syscall"
    //"encoding/binary"
    //"bytes"
    //"io"
)

const defRobotNum = 1



const  (
    //网关状态
    kStateIdle = iota
    kStateReqLogining
    kStateReqRegister
    kStateLogin
    kStateMax
)

type C2GLogon  struct {
    name  string
    pwd   string 
}

type  STATE_CB  func(  user *User ) 
type User struct {
    name    string
    pwd     string 
    
    state   int
    
    r       *robot.Robot 
    
    stateCbs [kStateMax]STATE_CB
}

func  ( u *User ) RegisterCb() {
    
    u.stateCbs[kStateIdle]        = onIdle
    u.stateCbs[kStateReqLogining] = onReqLogining
    //u.stateCbs[kStateReqRegister] = onReqRegister
    //u.stateCbs[kStateLogin]       = onLogin
}

func  onIdle( u *User ) {
    
    var msg robot.Message
    
    //binary.Write( buf, binary.LittleEndian,  u.name )
    //binary.Write( buf, binary.LittleEndian,  u.pwd )
    
    msg.Data = make( []byte, 96 )
   
    userName := msg.Data[0:64]
    pwd := msg.Data[64:]
    
    copy( userName, []byte(u.name))
    copy( pwd, []byte(u.pwd))
    msg.SetId(1)
    u.r.Send( msg )
    
    elog.LogInfo(" send msg :%d ", len(msg.Data))
    u.state = kStateReqLogining
}


func onReqLogining( u *User ) {
    
    msg , err := u.r.GetMsg() 
    if err != nil {
        //elog.LogInfo(" get msg fail ")
        return 
    }else {
        elog.LogInfo(" i receieve msg :%d ", msg.GetId() )
    }
}
func process(r *robot.Robot) {
    
    if r.UserData == nil {
        
        elog.LogInfoln( " user data ", r.UserData )
        r.UserData = &User{
            name  :  strconv.Itoa( int(r.GetId()) ),
            pwd   : "123",
            state : kStateIdle,
            r     : r,
        }
        user := r.UserData.( *User )
        user.RegisterCb()
    }

    //elog.LogInfoln( " user data ", r.UserData )
    user := r.UserData.( *User )
    user.stateCbs[user.state ]( user )
}

func main() {

    elog.InitLog( elog.INFO )
	elog.LogInfo(" --------------------- let go ------------------------")
	robotMng := robot.NewRobotMng()
	robotMng.SetUpdateCb(process)
	robotMng.Run()
    //信号处理
	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	robotMng.Close()

}
