package server

import (
	"context"
	//"crypto/tls"
	//"crypto/x509"
	//"errors"
	"fmt"
	//"google.golang.org/grpc"
	//"google.golang.org/grpc/credentials"
	//"io/ioutil"
	"net"

	"github.com/Ferlab-Ste-Justine/systemd-remote/config"
	"github.com/Ferlab-Ste-Justine/systemd-remote/logger"
	"github.com/Ferlab-Ste-Justine/systemd-remote/units"

	//"github.com/Ferlab-Ste-Justine/etcd-sdk/keypb"
)

//Idea: internal error threshold on responses if errors are not input errors, configurable
func Serve(serverConf config.ServerConfig, man units.UnitsManager, log logger.Logger) (context.CancelFunc, <-chan error) {
	/*ctx*/_, cancel := context.WithCancel(context.Background())
	errChan := make(chan error)

	go func() {
		defer close(errChan)

		/*listener*/_, err := net.Listen("tcp", fmt.Sprintf("%s:%d", serverConf.BindIp, serverConf.Port))
		if err != nil {
			errChan <- err
			return
		}
		
		//TODO: Finish
	}()

	return cancel, errChan
}