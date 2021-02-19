package redis

import (
	"github.com/apex/log"
	"github.com/cenkalti/backoff/v4"
	"github.com/crawlab-team/crawlab-db/errors"
	"github.com/crawlab-team/crawlab-db/utils"
	"github.com/crawlab-team/go-trace"
	"github.com/gomodule/redigo/redis"
	"github.com/spf13/viper"
	"reflect"
	"strings"
	"time"
)

var RedisClient *Redis

var MemoryStatsMetrics = []string{
	"peak.allocated",
	"total.allocated",
	"startup.allocated",
	"overhead.total",
	"keys.count",
	"dataset.bytes",
}

type Redis struct {
	pool *redis.Pool
}

type Mutex struct {
	Name   string
	expiry time.Duration
	tries  int
	delay  time.Duration
	value  string
}

func NewRedisClient() *Redis {
	return &Redis{pool: NewRedisPool()}
}

func (r *Redis) Del(collection string) error {
	c := r.pool.Get()
	defer utils.Close(c)

	if _, err := c.Do("DEL", collection); err != nil {
		return trace.TraceError(err)
	}
	return nil
}

func (r *Redis) LLen(collection string) (int, error) {
	c := r.pool.Get()
	defer utils.Close(c)

	value, err := redis.Int(c.Do("LLEN", collection))
	if err != nil {
		return 0, trace.TraceError(err)
	}
	return value, nil
}

func (r *Redis) RPush(collection string, value interface{}) error {
	c := r.pool.Get()
	defer utils.Close(c)

	if _, err := c.Do("RPUSH", collection, value); err != nil {
		return trace.TraceError(err)
	}
	return nil
}

func (r *Redis) LPush(collection string, value interface{}) error {
	c := r.pool.Get()
	defer utils.Close(c)

	if _, err := c.Do("RPUSH", collection, value); err != nil {
		if err != redis.ErrNil {
			return trace.TraceError(err)
		}
		return err
	}
	return nil
}

func (r *Redis) LPop(collection string) (string, error) {
	c := r.pool.Get()
	defer utils.Close(c)

	value, err := redis.String(c.Do("LPOP", collection))
	if err != nil {
		if err != redis.ErrNil {
			return value, trace.TraceError(err)
		}
		return value, err
	}
	return value, nil
}

func (r *Redis) HSet(collection string, key string, value string) error {
	c := r.pool.Get()
	defer utils.Close(c)

	if _, err := c.Do("HSET", collection, key, value); err != nil {
		if err != redis.ErrNil {
			return trace.TraceError(err)
		}
		return err
	}
	return nil
}

func (r *Redis) Ping() error {
	c := r.pool.Get()
	defer utils.Close(c)
	if _, err := redis.String(c.Do("PING")); err != nil {
		if err != redis.ErrNil {
			return trace.TraceError(err)
		}
		return err
	}
	return nil
}

func (r *Redis) HGet(collection string, key string) (string, error) {
	c := r.pool.Get()
	defer utils.Close(c)
	value, err := redis.String(c.Do("HGET", collection, key))
	if err != nil && err != redis.ErrNil {
		if err != redis.ErrNil {
			return value, trace.TraceError(err)
		}
		return value, err
	}
	return value, nil
}

func (r *Redis) HDel(collection string, key string) error {
	c := r.pool.Get()
	defer utils.Close(c)

	if _, err := c.Do("HDEL", collection, key); err != nil {
		return trace.TraceError(err)
	}
	return nil
}

func (r *Redis) HScan(collection string) (results []string, err error) {
	c := r.pool.Get()
	defer utils.Close(c)
	var (
		cursor int64
		items  []string
	)

	for {
		values, err := redis.Values(c.Do("HSCAN", collection, cursor))
		if err != nil {
			if err != redis.ErrNil {
				return results, trace.TraceError(err)
			}
			return results, err
		}

		values, err = redis.Scan(values, &cursor, &items)
		if err != nil {
			if err != redis.ErrNil {
				return results, trace.TraceError(err)
			}
			return results, err
		}
		for i := 0; i < len(items); i += 2 {
			cur := items[i+1]
			results = append(results, cur)
		}
		if cursor == 0 {
			break
		}
	}
	return results, nil
}

func (r *Redis) HKeys(collection string) (results []string, err error) {
	c := r.pool.Get()
	defer utils.Close(c)

	results, err = redis.Strings(c.Do("HKEYS", collection))
	if err != nil {
		if err != redis.ErrNil {
			return results, trace.TraceError(err)
		}
		return results, err
	}
	return results, nil
}

func (r *Redis) BRPop(collection string, timeout int) (value string, err error) {
	if timeout <= 0 {
		timeout = 60
	}
	c := r.pool.Get()
	defer utils.Close(c)

	values, err := redis.Strings(c.Do("BRPOP", collection, timeout))
	if err != nil {
		if err != redis.ErrNil {
			return value, trace.TraceError(err)
		}
		return value, err
	}
	return values[1], nil
}

func NewRedisPool() *redis.Pool {
	var address = viper.GetString("redis.address")
	var port = viper.GetString("redis.port")
	var database = viper.GetString("redis.database")
	var password = viper.GetString("redis.password")

	// normalize params
	if address == "" {
		address = "localhost"
	}
	if port == "" {
		port = "6379"
	}
	if database == "" {
		database = "1"
	}

	var url string
	if password == "" {
		url = "redis://" + address + ":" + port + "/" + database
	} else {
		url = "redis://x:" + password + "@" + address + ":" + port + "/" + database
	}
	return &redis.Pool{
		Dial: func() (conn redis.Conn, e error) {
			return redis.DialURL(url,
				redis.DialConnectTimeout(time.Second*10),
				redis.DialReadTimeout(time.Second*600),
				redis.DialWriteTimeout(time.Second*10),
			)
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if time.Since(t) < time.Minute {
				return nil
			}
			_, err := c.Do("PING")
			return trace.TraceError(err)
		},
		MaxIdle:         10,
		MaxActive:       0,
		IdleTimeout:     300 * time.Second,
		Wait:            false,
		MaxConnLifetime: 0,
	}
}

func InitRedis() error {
	RedisClient = NewRedisClient()
	b := backoff.NewExponentialBackOff()
	b.MaxInterval = 20 * time.Second
	err := backoff.Retry(func() error {
		err := RedisClient.Ping()

		if err != nil {
			log.WithError(err).Warnf("waiting for redis pool active connection. will after %f seconds try  again.", b.NextBackOff().Seconds())
		}
		return trace.TraceError(err)
	}, b)
	return trace.TraceError(err)
}

// 构建同步锁key
func (r *Redis) getLockKey(lockKey string) string {
	lockKey = strings.ReplaceAll(lockKey, ":", "-")
	return "nodes:lock:" + lockKey
}

// 获得锁
func (r *Redis) Lock(lockKey string) (value int64, err error) {
	c := r.pool.Get()
	defer utils.Close(c)
	lockKey = r.getLockKey(lockKey)

	ts := time.Now().Unix()
	ok, err := c.Do("SET", lockKey, ts, "NX", "PX", 30000)
	if err != nil {
		if err != redis.ErrNil {
			return value, trace.TraceError(err)
		}
		return value, err
	}
	if ok == nil {
		return 0, trace.TraceError(errors.ErrAlreadyLocked)
	}
	return ts, nil
}

func (r *Redis) UnLock(lockKey string, value int64) {
	c := r.pool.Get()
	defer utils.Close(c)
	lockKey = r.getLockKey(lockKey)

	getValue, err := redis.Int64(c.Do("GET", lockKey))
	if err != nil {
		log.Errorf("get lockKey error: %s", err.Error())
		return
	}

	if getValue != value {
		log.Errorf("the lockKey value diff: %d, %d", value, getValue)
		return
	}

	v, err := redis.Int64(c.Do("DEL", lockKey))
	if err != nil {
		log.Errorf("unlock failed, error: %s", err.Error())
		return
	}

	if v == 0 {
		log.Errorf("unlock failed: key=%s", lockKey)
		return
	}
}

func (r *Redis) MemoryStats() (stats map[string]int64, err error) {
	stats = map[string]int64{}
	c := r.pool.Get()
	defer utils.Close(c)
	values, err := redis.Values(c.Do("MEMORY", "STATS"))
	for i, v := range values {
		t := reflect.TypeOf(v)
		if t.Kind() == reflect.Slice {
			vc, _ := redis.String(v, err)
			if utils.ContainsString(MemoryStatsMetrics, vc) {
				stats[vc], _ = redis.Int64(values[i+1], err)
			}
		}
	}
	if err != nil {
		if err != redis.ErrNil {
			return stats, trace.TraceError(err)
		}
		return stats, err
	}
	return stats, nil
}
