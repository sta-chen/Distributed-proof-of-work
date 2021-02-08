package main

import (
	"flag"
	"log"

	"example.org/cpsc416/a2/powlib"

	distpow "example.org/cpsc416/a2"
)

func main() {
	var config distpow.ClientConfig
	err := distpow.ReadJSONConfig("config/client_config.json", &config)
	if err != nil {
		log.Fatal(err)
	}
	flag.StringVar(&config.ClientID, "id", config.ClientID, "Client ID, e.g. client1")
	flag.Parse()

	client := distpow.NewClient(config, powlib.NewPOW())
	if err := client.Initialize(); err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	if err := client.Mine([]uint8{1, 2, 3, 4}, 7); err != nil {
		log.Println(err)
	}
	if err := client.Mine([]uint8{5, 6, 7, 8}, 5); err != nil {
		log.Println(err)
	}

	for i := 0; i < 2; i++ {
		mineResult := <-client.NotifyChannel
		log.Println(mineResult)
	}
}