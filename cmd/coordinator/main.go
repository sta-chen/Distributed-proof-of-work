package main

import (
	distpow "example.org/cpsc416/a2"
	"github.com/DistributedClocks/tracing"
	"log"
	"net"
	"net/http"
	"net/rpc"
)

type Coordinator struct {
	Tracer         *tracing.Tracer
	numWorkers     uint
	Workers        []*rpc.Client
	config         distpow.CoordinatorConfig
	resultChan     chan distpow.CoordinatorWorkerResult
	resultReceived chan bool
}

func (co *Coordinator) Mine(args *distpow.CoordinatorMine, reply *[]uint8) error {
	co.Tracer.RecordAction(distpow.CoordinatorMine{
		Nonce:            args.Nonce,
		NumTrailingZeros: args.NumTrailingZeros,
	})
	threadCount := int(co.numWorkers)

	workerConnected := 0
	co.Workers = make([]*rpc.Client, len(co.config.Workers))
	for {
		if workerConnected == int(co.numWorkers) {
			break
		}
		for index, worker := range co.config.Workers {
			if co.Workers[index] == nil {
				var e error
				co.Workers[index], e = rpc.DialHTTP("tcp", string(worker))
				if e == nil {
					workerConnected++
				}
			}
		}

	}

	killChan := make(chan struct{}, threadCount)
	co.resultChan = make(chan distpow.CoordinatorWorkerResult, threadCount)
	co.resultReceived = make(chan bool, 1)

	for i := 0; i < threadCount; i++ {
		go miner(co.Tracer, args.Nonce, uint8(i), args.NumTrailingZeros, co.Workers[i], co.numWorkers)
	}
	result := <-co.resultChan
	for i := 0; i < threadCount; i++ {
		if i != int(result.WorkerByte) {
			go cancel(co.Tracer, args.Nonce, uint8(i), args.NumTrailingZeros, killChan, co.Workers[i])
		}
	}
	for i := 0; i < threadCount-1; i++ {
		<-killChan
	}
	close(co.resultChan)
	co.Tracer.RecordAction(distpow.CoordinatorSuccess{
		Nonce:            args.Nonce,
		NumTrailingZeros: args.NumTrailingZeros,
		Secret:           result.Secret,
	})
	*reply = result.Secret
	return nil
}

func miner(tracer *tracing.Tracer, nonce []uint8, threadByte uint8, trailingZeroesSearch uint,
	workerClient *rpc.Client, numWorkers uint) {
	var result distpow.WorkerResult
	args := &distpow.Args{
		Nonce:            nonce,
		NumTrailingZeros: trailingZeroesSearch,
		WorkerByte:       threadByte,
		NumWorkers:       numWorkers,
	}
	// Asynchronous call
	tracer.RecordAction(distpow.CoordinatorWorkerMine{
		Nonce:            nonce,
		NumTrailingZeros: trailingZeroesSearch,
		WorkerByte:       threadByte,
	})
	workerMine := workerClient.Go("Worker.Mine", args, &result, nil)
	<-workerMine.Done
}

func cancel(tracer *tracing.Tracer, nonce []uint8, threadByte uint8, trailingZeroesSearch uint, killChan chan struct{},
	workerClient *rpc.Client) {
	var result struct{}
	args := &distpow.WorkerCancel{
		Nonce:            nonce,
		NumTrailingZeros: trailingZeroesSearch,
		WorkerByte:       threadByte,
	}
	tracer.RecordAction(distpow.CoordinatorWorkerCancel{
		Nonce:            nonce,
		NumTrailingZeros: trailingZeroesSearch,
		WorkerByte:       threadByte,
	})
	workerCancel := workerClient.Go("Worker.Cancel", args, &result, nil)
	<-workerCancel.Done
	killChan <- result
}

func (co *Coordinator) Result(args *distpow.CoordinatorWorkerResult, reply *error) error {
	result := distpow.CoordinatorWorkerResult{
		Nonce:            args.Nonce,
		NumTrailingZeros: args.NumTrailingZeros,
		WorkerByte:       args.WorkerByte,
		Secret:           args.Secret,
	}
	co.Tracer.RecordAction(result)
	select {
	case _, ok := <-co.resultReceived:
		if ok {
			close(co.resultReceived)
		}
		*reply = nil
		return nil
	default:
		// pass
	}
	co.resultReceived <- true
	co.resultChan <- result
	*reply = nil
	return nil
}

func main() {
	var config distpow.CoordinatorConfig
	err := distpow.ReadJSONConfig("config/coordinator_config.json", &config)
	if err != nil {
		log.Fatal(err)
	}

	log.Println(config)

	coordinator := new(Coordinator)
	tracerConfig := tracing.TracerConfig{
		ServerAddress:  config.TracerServerAddr,
		TracerIdentity: "coordinator",
		Secret:         config.TracerSecret,
	}

	coordinator.Tracer = tracing.NewTracer(tracerConfig)
	coordinator.numWorkers = uint(len(config.Workers))
	coordinator.config = config

	rpc.Register(coordinator)
	rpc.HandleHTTP()
	clientL, clientE := net.Listen("tcp", config.ClientAPIListenAddr)
	if clientE != nil {
		log.Println(clientE)
	}
	go http.Serve(clientL, nil)

	workerL, workerE := net.Listen("tcp", config.WorkerAPIListenAddr)
	if workerE != nil {
		log.Println(workerE)
	}
	go http.Serve(workerL, nil)
	for {
	}
}
