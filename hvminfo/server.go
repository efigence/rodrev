package hvminfo

import (
	"encoding/json"
	"go.uber.org/zap"
	"net"
	"time"
)

type ConfigServer struct {
	Listen string `json:"listen"`
	Info HVMInfo `json:"hvm_info,omitempty"`
	Logger *zap.SugaredLogger `yaml:"-"`
}

func Run(c ConfigServer) {
    udpAddr, err := net.ResolveUDPAddr("udp", c.Listen)
    if err != nil {
    	c.Logger.Errorf("error resolving address %s: %s, will restart in hour to check whether problem is fixed",c.Logger,err)
		time.Sleep(time.Hour)
    	c.Logger.Panicf("error resolving address %s: %s, restarting",c.Logger,err)
	}


    conn, err := net.ListenUDP("udp", udpAddr)
    if err != nil {
    	c.Logger.Errorf("error listening on %s: %s, will restart in hour to check whether problem is fixed",c.Logger,err)
		time.Sleep(time.Hour)
    	c.Logger.Panicf("error listening on %s: %s, restarting",c.Logger,err)
	}
	c.Logger.Infof("starting UDP listener for hvminfo on %s", udpAddr.String())
    // TODO update that if we ever get anything dynamic here
	data, err := json.Marshal(&c.Info)
	data = append(data,[]byte("\n")...);
	if err != nil {
		c.Logger.Warnf("error marshalling json: %s", err)
	}
	for {

		var buf [1500]byte

		_, addr, err := conn.ReadFromUDP(buf[0:])
		if err != nil {
			c.Logger.Errorf("error receiving packet %s: %s, will restart in hour to check whether problem is fixed",addr.String(),err)
			time.Sleep(time.Hour)
			c.Logger.Panicf("error receiving packet %s: %s, restarting",c.Logger,err)
		}
		_, err = conn.WriteToUDP(data, addr)
		if err != nil {c.Logger.Warnf("error when sending packet to %s: %s", addr.String(), err)}
    }

}