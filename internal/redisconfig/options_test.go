package redisconfig

import "testing"

func TestOptionsBuildsConsistentClients(t *testing.T) {
	options := Options{
		Addr:     " redis.example.com:6379 ",
		Username: " app ",
		Password: "secret",
		DB:       3,
	}

	goRedis := options.GoRedis()
	asynq := options.Asynq()
	if goRedis.Addr != asynq.Addr || goRedis.Username != asynq.Username || goRedis.Password != asynq.Password || goRedis.DB != asynq.DB {
		t.Fatalf("go-redis and Asynq options differ: %#v %#v", goRedis, asynq)
	}
	if goRedis.Addr != "redis.example.com:6379" || goRedis.Username != "app" {
		t.Fatalf("options were not normalized: %#v", goRedis)
	}
}

func TestOptionsNormalizesNegativeDatabase(t *testing.T) {
	if got := (Options{DB: -1}).Normalized().DB; got != 0 {
		t.Fatalf("DB = %d, want 0", got)
	}
}
