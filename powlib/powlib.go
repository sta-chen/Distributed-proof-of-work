// Package powlib provides an API which is a wrapper around RPC calls to the
// coordinator.
package powlib

import (
	"errors"
	"github.com/DistributedClocks/tracing"
	"net/rpc"
)

type PowlibMiningBegin struct {
	Nonce            []uint8
	NumTrailingZeros uint
}

type PowlibMine struct {
	Nonce            []uint8
	NumTrailingZeros uint
}

type PowlibSuccess struct {
	Nonce            []uint8
	NumTrailingZeros uint
	Secret           []uint8
}

type PowlibMiningComplete struct {
	Nonce            []uint8
	NumTrailingZeros uint
	Secret           []uint8
}

// MineResult contains the result of a mining request.
type MineResult struct {
	Nonce            []uint8
	NumTrailingZeros uint
	Secret           []uint8
}

// NotifyChannel is used for notifying the client about a mining result.
type NotifyChannel chan MineResult

type Args struct {
	Tracer           *tracing.Tracer
	Nonce            []uint8
	NumTrailingZeros uint
}

// POW struct represents an instance of the powlib.
type POW struct {
	Notifications NotifyChannel
	coordAddr     string
	connection    *rpc.Client
}

func NewPOW() *POW {
	return &POW{
		Notifications: nil,
		coordAddr:     "",
		connection:    nil,
	}
}

// Initialize Initializes the instance of POW to use for connecting to the coordinator,
// and the coordinators IP:port. The returned notify-channel channel must
// have capacity ChCapacity and must be used by powlib to deliver all solution
// notifications. If there is an issue with connecting, this should return
// an appropriate err value, otherwise err should be set to nil.
func (d *POW) Initialize(coordAddr string, chCapacity uint) (NotifyChannel, error) {
	d.Notifications = make(NotifyChannel, chCapacity)
	d.coordAddr = coordAddr
	connection, err := rpc.DialHTTP("tcp", coordAddr)
	if err != nil {
		return nil, errors.New("dialing: " + err.Error())
	}
	d.connection = connection
	return d.Notifications, nil
}

// Mine is a non-blocking request from the client to the system solve a proof
// of work puzzle. The arguments have identical meaning as in A2. In case
// there is an underlying issue (for example, the coordinator cannot be reached),
// this should return an appropriate err value, otherwise err should be set to nil.
// Note that this call is non-blocking, and the solution to the proof of work
// puzzle must be delivered asynchronously to the client via the notify-channel
// channel returned in the Initialize call.
func (d *POW) Mine(tracer *tracing.Tracer, nonce []uint8, numTrailingZeros uint) error {
	tracer.RecordAction(PowlibMiningBegin{
		Nonce:            nonce,
		NumTrailingZeros: numTrailingZeros,
	})
	err := make(chan error)
	go mineHelper(tracer, nonce, numTrailingZeros, err, d.connection, d.Notifications)
	retError := <-err
	if retError != nil {
		return retError
	}
	secret := <-d.Notifications
	tracer.RecordAction(PowlibMiningComplete{
		Nonce:            nonce,
		NumTrailingZeros: numTrailingZeros,
		Secret:           secret.Secret,
	})
	return nil
}

func mineHelper(tracer *tracing.Tracer, nonce []uint8, numTrailingZeros uint, errChan chan error,
	connection *rpc.Client, notify NotifyChannel) {
	var secret []uint8
	args := &PowlibMine{Nonce: nonce, NumTrailingZeros: numTrailingZeros}
	tracer.RecordAction(PowlibMine{
		Nonce:            nonce,
		NumTrailingZeros: numTrailingZeros,
	})
	err := connection.Call("Coordinator.Mine", args, &secret)
	if err != nil {
		errChan <- err
		return
	}
	var result MineResult
	result.Nonce = nonce
	result.NumTrailingZeros = numTrailingZeros
	result.Secret = secret
	notify <- result
	tracer.RecordAction(PowlibSuccess{
		Nonce:            nonce,
		NumTrailingZeros: numTrailingZeros,
		Secret:           secret,
	})
	errChan <- nil
}

// Close Stops the POW instance from communicating with the coordinator and
// from delivering any solutions via the notify-channel. If there is an issue
// with stopping, this should return an appropriate err value, otherwise err
// should be set to nil.
func (d *POW) Close() error {
	if err := d.connection.Close(); err != nil {
		return errors.New("POW close: " + err.Error())
	}
	close(d.Notifications)
	return nil
}
