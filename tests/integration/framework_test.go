package integration

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	testDB     *pgxpool.Pool
	testMongo  *mongo.Client
	testCH     clickhouse.Conn
	mtlsClient *http.Client
)

func TestMain(m *testing.M) {
	// 1. Start Infrastructure
	fmt.Println("Starting infrastructure via Docker Compose...")
	cmd := exec.Command("docker", "compose", "-f", "../../infra/docker/docker-compose.yml", "up", "-d")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("failed to start infrastructure: %v", err)
	}

	// 2. Wait for services to be healthy
	fmt.Println("Waiting for services to be healthy...")
	var err error
	mtlsClient, err = NewMTLSClient("../../infra/certs/ca.crt")
	if err != nil {
		log.Fatalf("failed to create mTLS client: %v", err)
	}

	endpoints := []string{
		"http://localhost:8080/health",
		"https://localhost:8081/health",
		"https://localhost:8082/health",
		"https://localhost:8083/health",
		"https://localhost:8085/health",
	}

	for _, url := range endpoints {
		waitForHealth(url)
	}

	// 3. Initialize DB connections for verification
	ctx := context.Background()
	testDB, err = pgxpool.New(ctx, "postgres://openguard:openguard@localhost:5432/openguard?sslmode=disable")
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}

	testMongo, err = mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		log.Fatalf("failed to connect to mongodb: %v", err)
	}

	testCH, err = clickhouse.Open(&clickhouse.Options{
		Addr: []string{"localhost:9000"},
		Auth: clickhouse.Auth{
			Database: "openguard",
			Username: "default",
			Password: "",
		},
	})
	if err != nil {
		log.Fatalf("failed to connect to clickhouse: %v", err)
	}

	// 4. Run tests
	code := m.Run()

	os.Exit(code)
}

func waitForHealth(url string) {
	for i := 0; i < 30; i++ {
		resp, err := mtlsClient.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			fmt.Printf("Service at %s is healthy\n", url)
			return
		}
		fmt.Printf("Waiting for %s... (%d/30)\n", url, i+1)
		time.Sleep(2 * time.Second)
	}
	log.Fatalf("service at %s failed to become healthy", url)
}

func Eventually(t *testing.T, fn func() bool, timeout time.Duration, interval time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(interval)
	}
	t.Errorf("condition not met within %v", timeout)
}
