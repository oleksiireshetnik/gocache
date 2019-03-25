package impl

import (
	"strconv"
	"time"

	"github.com/getupandgo/gocache/common/structs"
	"github.com/go-redis/redis"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type RedisClient struct {
	*redis.Client
}

func Init() (*RedisClient, error) {
	host := viper.GetString("redis.host")
	port := viper.GetString("redis.port")

	rc := &RedisClient{}
	rc.Client = redis.NewClient(&redis.Options{
		Addr: host + ":" + port,
		DB:   0,
	})

	_, err := rc.Ping().Result()
	if err != nil {
		return nil, err
	}

	return rc, nil

}

func (db *RedisClient) Get(url string) ([]byte, error) {
	pipe := db.TxPipeline()

	pipe.ZIncr("hits", redis.Z{
		Score:  1,
		Member: url,
	})

	content, err := db.HGet(url, "content").Bytes()
	if err != nil {
		return nil, err
	}

	_, err = pipe.Exec()
	if err != nil {
		return nil, err
	}

	return content, nil
}

func (db *RedisClient) Upsert(pg *structs.Page) (bool, error) {
	err := db.evictIfFull(pg.TotalSize)

	var setCommand *redis.BoolCmd
	var upsertPage func(string) error

	upsertPage = func(url string) error {
		upsertTx := func(tx *redis.Tx) error {
			_, err = tx.Pipelined(
				func(pipe redis.Pipeliner) error {
					setCommand = pipe.HSet(pg.URL, "content", pg.Content)

					pipe.ZIncr("hits", redis.Z{
						Score:  1,
						Member: pg.URL,
					})

					pipe.ZAdd("ttl", redis.Z{
						Score:  float64(pg.TTL),
						Member: pg.URL,
					})

					return err
				})

			return err
		}

		err := db.Watch(upsertTx, url)

		if err == redis.TxFailedErr {
			log.Warn().
				Err(err).
				Msg("Failed to insert page with url " + url + ", retry")

			return upsertPage(url)
		}

		return err
	}

	err = upsertPage(pg.URL)
	if err != nil {
		return false, err
	}

	return setCommand.Result()
}

func (db *RedisClient) Top() ([]structs.ScoredPage, error) {
	topPagesNum := viper.GetInt64("cache.top_records_number")

	topPages, err := db.ZRevRangeWithScores("hits", 0, topPagesNum-1).Result()
	if err != nil {
		return nil, err
	}

	return parseZ(&topPages), nil
}

func (db *RedisClient) Remove(url string) (int, error) {
	var memoryUsageCommand *redis.IntCmd
	var removePage func(string) error

	removePage = func(url string) error {
		removeTx := func(tx *redis.Tx) error {
			_, err := tx.Pipelined(
				func(pipe redis.Pipeliner) error {
					memoryUsageCommand = pipe.MemoryUsage(url)

					pipe.HDel(url, "content")
					pipe.ZRem("hits", url)
					pipe.ZRem("ttl", url)

					return nil

				})

			return err
		}

		err := db.Watch(removeTx, url)

		if err == redis.TxFailedErr {
			log.Warn().
				Err(err).
				Msg("Failed to remove page with url " + url + ", retry")

			return removePage(url)
		}

		return err
	}

	err := removePage(url)
	if err != nil {
		return 0, err
	}

	bytesFreed, err := memoryUsageCommand.Result()

	return int(bytesFreed), err
}

func (db *RedisClient) Expire() (int, error) {
	nowUnix := time.Now().Unix()

	sPages, err := db.ZRangeByScore("ttl", redis.ZRangeBy{
		Min: "0",
		Max: strconv.Itoa(int(nowUnix)),
	}).Result()

	if err != nil {
		return 0, err
	}

	var freedTotal int

	for _, sPage := range sPages {
		sizeFreed, err := db.Remove(sPage)
		if err != nil {
			log.Warn().
				Err(err).
				Msg("Failed to remove expired items")

			continue
		}

		freedTotal += sizeFreed
	}

	return freedTotal, err
}
