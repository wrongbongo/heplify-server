package elastic

import (
	"context"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/negbie/heplify-server"
	"github.com/negbie/heplify-server/config"
	"github.com/negbie/logp"
	"github.com/olivere/elastic"
)

type Elasticsearch struct {
	bulkClient *elastic.BulkProcessor
	indexName  string
}

func (e *Elasticsearch) setup() error {
	var err error
	var client *elastic.Client
	ctx := context.Background()
	e.indexName = "heplify-server"
	for {
		client, err = elastic.NewClient(
			elastic.SetURL(config.Setting.ESAddr),
			elastic.SetSniff(false),
		)
		if err != nil {
			logp.Err("%v", err)
			time.Sleep(5 * time.Second)
		} else {
			break
		}
	}
	e.bulkClient, err = client.BulkProcessor().
		Name("ESBulkProcessor").
		Workers(runtime.NumCPU()).
		BulkActions(1000).
		BulkSize(2 << 20).
		FlushInterval(10 * time.Second).
		Do(ctx)
	if err != nil {
		return err
	}
	// Use the IndexExists service to check if a specified index exists.
	exists, err := client.IndexExists(e.indexName).Do(ctx)
	if err != nil {
		return err
	}
	if !exists {
		// Create a new index.
		createIndex, err := client.CreateIndex(e.indexName).Do(ctx)
		if err != nil {
			return err
		}
		if !createIndex.Acknowledged {
			// Not acknowledged
		}
	}
	return nil
}

func (e *Elasticsearch) send(hCh chan *decoder.HEP) {
	var (
		pkt *decoder.HEP
		ok  bool
	)

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case pkt, ok = <-hCh:
			if !ok {
				break
			}
			r := elastic.NewBulkIndexRequest().Index(e.indexName).Type("hep").Doc(pkt)
			e.bulkClient.Add(r)

		case <-c:
			logp.Info("heplify-server wants to stop flush remaining es bulk index requests")
			e.bulkClient.Flush()
		}
	}
}
