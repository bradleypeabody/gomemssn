package gomemssn

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
)

const (
	testMemcacheServer = "127.0.0.1:11211"
)

var httpClient = &http.Client{}

func init() {
	jar, _ := cookiejar.New(nil)
	httpClient.Jar = jar
}

func mustGet(url string) []byte {
	resp, err := httpClient.Get(url)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	return body
}

var manager *Manager

var formHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

	ssn := manager.MustSession(w, r)
	defer manager.MustWriteSession(w, ssn)

	v := r.FormValue("v")
	if v != "" {
		ssn.Values["v"] = v
	} else {
		vo := ssn.Values["v"]
		if v1, ok := vo.(string); ok {
			v = v1
		}
	}

	fmt.Fprint(w, v)

})

func TestBasic(t *testing.T) {

	fmt.Printf("TestBasic\n")

	conn, err := net.Dial("tcp", testMemcacheServer)
	if err != nil {
		t.Logf("No memcache running locally (%v), skipping this test", testMemcacheServer)
		t.SkipNow()
	} else {
		conn.Close()
	}

	memcacheClient := memcache.New(testMemcacheServer)
	sm := NewManager(memcacheClient, "gomemssn_test")
	manager = sm

	s := &http.Server{Handler: formHandler}

	l, err := net.Listen("tcp", "127.0.0.1:18080")
	if err != nil {
		panic(err)
	}
	defer l.Close()
	go s.Serve(l)

	v := string(mustGet("http://127.0.0.1:18080/"))
	fmt.Printf("v=%s\n", v)
	if v != "" {
		t.Fatalf("expected v='' but got: %v", v)
	}

	v = string(mustGet("http://127.0.0.1:18080/?v=abc123"))
	fmt.Printf("v=%s\n", v)
	if v != "abc123" {
		t.Fatalf("expected v='abc123' but got: %v", v)
	}

	v = string(mustGet("http://127.0.0.1:18080/"))
	fmt.Printf("v=%s\n", v)
	if v != "abc123" {
		t.Fatalf("expected v='abc123' but got: %v", v)
	}

}

// test the stub functionality
func TestStub(t *testing.T) {

	fmt.Printf("TestStub\n")

	sm := NewManager(nil, "gomemssn_test")
	manager = sm

	s := &http.Server{Handler: formHandler}

	l, err := net.Listen("tcp", "127.0.0.1:18080")
	if err != nil {
		panic(err)
	}
	defer l.Close()
	go s.Serve(l)

	v := string(mustGet("http://127.0.0.1:18080/"))
	fmt.Printf("v=%s\n", v)
	if v != "" {
		t.Fatalf("expected v='' but got: %v", v)
	}

	v = string(mustGet("http://127.0.0.1:18080/?v=abc123"))
	fmt.Printf("v=%s\n", v)
	if v != "abc123" {
		t.Fatalf("expected v='abc123' but got: %v", v)
	}

	v = string(mustGet("http://127.0.0.1:18080/"))
	fmt.Printf("v=%s\n", v)
	if v != "abc123" {
		t.Fatalf("expected v='abc123' but got: %v", v)
	}

}

// test the expiration functionality
func TestExpiration(t *testing.T) {

	fmt.Printf("TestExpiration\n")

	conn, err := net.Dial("tcp", testMemcacheServer)
	if err != nil {
		t.Logf("No memcache running locally (%v), skipping this test", testMemcacheServer)
		t.SkipNow()
	} else {
		conn.Close()
	}

	memcacheClient := memcache.New(testMemcacheServer)
	sm := NewManager(memcacheClient, "gomemssn_test")
	sm.Expiration = time.Second * 2
	manager = sm

	s := &http.Server{Handler: formHandler}

	l, err := net.Listen("tcp", "127.0.0.1:18080")
	if err != nil {
		panic(err)
	}
	defer l.Close()
	go s.Serve(l)

	mustGet("http://127.0.0.1:18080/?v=abc123")

	v := string(mustGet("http://127.0.0.1:18080/"))
	fmt.Printf("v=%s\n", v)
	if v != "abc123" {
		t.Fatalf("expected v='abc123' but got: %v", v)
	}

	// wait past the expiration
	time.Sleep(time.Second * 3)

	v = string(mustGet("http://127.0.0.1:18080/"))
	fmt.Printf("v=%s\n", v)
	if v != "" {
		t.Fatalf("expected v='' but got: %v", v)
	}

}
