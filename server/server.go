package server

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/credentials"
	"io"
	"io/ioutil"
	"net"

	"github.com/Ferlab-Ste-Justine/systemd-remote/config"
	"github.com/Ferlab-Ste-Justine/systemd-remote/logger"
	"github.com/Ferlab-Ste-Justine/systemd-remote/units"

	"github.com/Ferlab-Ste-Justine/etcd-sdk/keypb"
)

func getTlsConfig(opts config.ServerTlsConfig) (credentials.TransportCredentials, error) {
	//Server Certificate
	certData, err := tls.LoadX509KeyPair(opts.ServerCert, opts.ServerKey)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Failed to load server certificates: %s", err.Error()))
	}

	//CA Certificate
	caCertContent, err := ioutil.ReadFile(opts.CaCert)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Failed to read ca certificate file: %s", err.Error()))
	}
	ca := x509.NewCertPool()
	ok := ca.AppendCertsFromPEM(caCertContent)
	if !ok {
		return nil, errors.New("Failed to parse ca certificate authority")
	}

	//Tls confs
	tlsConf := &tls.Config{
		ClientAuth: tls.RequireAnyClientCert,
		InsecureSkipVerify: false,
		Certificates: []tls.Certificate{certData},
		ClientCAs: ca,
	}

	return credentials.NewTLS(tlsConf), nil
}


type Server struct {
	keypb.KeyPushServiceServer
	Manager units.UnitsManager
}

func (s *Server) SendKeyDiff(stream keypb.KeyPushService_SendKeyDiffServer) error {
	reqCh := make(chan *keypb.SendKeyDiffRequest)
	defer close(reqCh)
	keyDiffCh := keypb.ProcessSendKeyDiffRequests(reqCh)
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			close(reqCh)
			break
		}
		if err != nil {
			return err
		}

		select {
		case result := <- keyDiffCh:
			code := codes.Unknown
			if keypb.IsApiContractError(result.Error) {
				code = codes.InvalidArgument
			}
			return status.New(code, result.Error.Error()).Err()
		default:
		}

		reqCh <- req
	}

	result := <- keyDiffCh
	if result.Error != nil {
		code := codes.Unknown
		if keypb.IsApiContractError(result.Error) {
			code = codes.InvalidArgument
		}
		return status.New(code, result.Error.Error()).Err()
	}

	applyErr := s.Manager.Apply(result.KeyDiff)
	if applyErr != nil {
		code := codes.Unknown
		if units.IsApiContractError(applyErr) {
			code = codes.InvalidArgument
		}
		return status.New(code, applyErr.Error()).Err()
	}

	return nil
}

type StopServer func()

//Idea: internal error threshold on responses if errors are not input errors, configurable
func Serve(serverConf config.ServerConfig, man units.UnitsManager, log logger.Logger) (StopServer, <-chan error) {
	errChan := make(chan error)
	var grpcServer *grpc.Server

	stopServerFn := func() {
		if grpcServer != nil {
			grpcServer.GracefulStop()
		}
	}

	go func() {
		defer close(errChan)

		listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", serverConf.BindIp, serverConf.Port))
		if err != nil {
			errChan <- err
			return
		}
		
		tlsConf, tlsErr := getTlsConfig(serverConf.Tls)
		if tlsErr != nil {
			errChan <- err
			return
		}

		var opts []grpc.ServerOption
		opts = append(opts, grpc.Creds(tlsConf))
	
		grpcServer := grpc.NewServer(opts...)
		keypb.RegisterKeyPushServiceServer(grpcServer, &Server{Manager: man})
		serveErr := grpcServer.Serve(listener)
		if serveErr != nil {
			errChan <- serveErr
		}
	}()

	return stopServerFn, errChan
}