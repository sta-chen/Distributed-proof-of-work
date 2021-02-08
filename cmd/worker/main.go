package main

import (
	"bytes"
	"crypto/md5"
	distpow "example.org/cpsc416/a2"
	"flag"
	"fmt"
	"github.com/DistributedClocks/tracing"
	"log"
	"math/bits"
	"net"
	"net/http"
	"net/rpc"
)

type Worker struct {
	Tracer            *tracing.Tracer
	Nonce             []uint8
	NumbTrailingZeros uint
	killChan          chan struct{}
	coordAddr         string
	calledResult      chan bool
}

func (w *Worker) Mine(args *distpow.Args, reply *distpow.WorkerResult) error {
	w.Tracer.RecordAction(distpow.WorkerMine{
		Nonce:            args.Nonce,
		NumTrailingZeros: args.NumTrailingZeros,
		WorkerByte:       args.WorkerByte,
	})
	threadByte := args.WorkerByte
	threadBits := uint8(bits.Len(args.NumWorkers) - 1)
	remainderBits := 8 - (threadBits % 8)

	w.calledResult = make(chan bool, 1)
	w.killChan = make(chan struct{}, 1)

	chunk := []uint8{}

	hashStrBuf, wholeBuffer := new(bytes.Buffer), new(bytes.Buffer)
	if _, err := wholeBuffer.Write(args.Nonce); err != nil {
		panic(err)
	}
	wholeBufferTrunc := wholeBuffer.Len()

	// table out all possible "thread bytes", aka the byte prefix
	// between the nonce and the bytes explored by this worker
	remainderEnd := 1 << remainderBits
	threadBytes := make([]uint8, remainderEnd)
	for i := 0; i < remainderEnd; i++ {
		threadBytes[i] = uint8((int(threadByte) << remainderBits) | i)
	}

	for {
		for _, threadByte := range threadBytes {
			// optional: end early
			select {
			case <-w.killChan:
				return nil
			default:
				// pass
			}
			wholeBuffer.Truncate(wholeBufferTrunc)
			if err := wholeBuffer.WriteByte(threadByte); err != nil {
				panic(err)
			}
			if _, err := wholeBuffer.Write(chunk); err != nil {
				panic(err)
			}
			hash := md5.Sum(wholeBuffer.Bytes())
			hashStrBuf.Reset()
			fmt.Fprintf(hashStrBuf, "%x", hash)
			if hasNumZeroesSuffix(hashStrBuf.Bytes(), args.NumTrailingZeros) {
				client, dialError := rpc.DialHTTP("tcp", w.coordAddr)
				if dialError != nil {
					log.Println(dialError)
				}
				resultArgs := &distpow.CoordinatorWorkerResult{
					Nonce:            args.Nonce,
					NumTrailingZeros: args.NumTrailingZeros,
					WorkerByte:       args.WorkerByte,
					Secret:           wholeBuffer.Bytes()[wholeBufferTrunc:],
				}
				var rep error
				w.Tracer.RecordAction(distpow.WorkerResult{
					Nonce:            args.Nonce,
					NumTrailingZeros: args.NumTrailingZeros,
					WorkerByte:       args.WorkerByte,
					Secret:           wholeBuffer.Bytes()[wholeBufferTrunc:],
				})
				w.calledResult <- true
				callCoordResult := client.Go("Coordinator.Result", resultArgs, &rep, nil)
				<-callCoordResult.Done
				return nil
			}
		}
		chunk = nextChunk(chunk)
	}
	return nil
}

func nextChunk(chunk []uint8) []uint8 {
	for i := 0; i < len(chunk); i++ {
		if chunk[i] == 0xFF {
			chunk[i] = 0
		} else {
			chunk[i]++
			return chunk
		}
	}
	return append(chunk, 1)
}

func hasNumZeroesSuffix(str []byte, numZeroes uint) bool {
	var trailingZeroesFound uint
	for i := len(str) - 1; i >= 0; i-- {
		if str[i] == '0' {
			trailingZeroesFound++
		} else {
			break
		}
	}
	return trailingZeroesFound >= numZeroes
}

func (w *Worker) Cancel(args *distpow.WorkerCancel, reply *error) error {

	select {
	case <-w.calledResult:
		w.Tracer.RecordAction(distpow.WorkerCancel{
			Nonce:            args.Nonce,
			NumTrailingZeros: args.NumTrailingZeros,
			WorkerByte:       args.WorkerByte,
		})
		*reply = nil
		return nil
	default:
		// pass
	}
	w.Tracer.RecordAction(distpow.WorkerCancel{
		Nonce:            args.Nonce,
		NumTrailingZeros: args.NumTrailingZeros,
		WorkerByte:       args.WorkerByte,
	})
	w.killChan <- struct{}{}
	*reply = nil
	return nil
}

func main() {
	var config distpow.WorkerConfig
	err := distpow.ReadJSONConfig("config/worker_config.json", &config)
	if err != nil {
		log.Fatal(err)
	}
	flag.StringVar(&config.WorkerID, "id", config.WorkerID, "Worker ID, e.g. worker1")
	flag.StringVar(&config.ListenAddr, "listen", config.ListenAddr, "Listen address, e.g. 127.0.0.1:5000")
	flag.Parse()

	log.Println(config)

	worker := new(Worker)
	tracerConfig := tracing.TracerConfig{
		ServerAddress:  config.TracerServerAddr,
		TracerIdentity: config.WorkerID,
		Secret:         config.TracerSecret,
	}
	worker.Tracer = tracing.NewTracer(tracerConfig)
	worker.coordAddr = config.CoordAddr

	rpc.Register(worker)
	rpc.HandleHTTP()

	l, e := net.Listen("tcp", config.ListenAddr)
	if e != nil {
		log.Println(e)
	}
	go http.Serve(l, nil)
	for {
	}
}
