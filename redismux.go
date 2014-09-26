package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var address = flag.String("listen", "0.0.0.0:6379", "The address the redis mux server listens on")
var verbose = flag.Bool("verbose", false, "Verbose output")
var profile = flag.String("profile", "", "write cpu profile to file")
var maxConnections = flag.Uint64("max_connections", 10000, "maximum number of open connection, will attempt to raise")

type RedisCommand struct {
	ParamCount int
	Params     []string
}

type RedisInfo struct {
	Pid  int
	Name string
	Cmd  *exec.Cmd
}

var redises = struct {
	sync.RWMutex
	m map[string]*RedisInfo
}{m: make(map[string]*RedisInfo)}

func main() {

	registerExitHandler()
	raiseUlimit()

	flag.Parse()

	if *profile != "" {
		f, err := os.Create(*profile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
	}

	files, _ := ioutil.ReadDir("dbs")
	for _, f := range files {
		if f.IsDir() {
			startupRedis(f.Name())
		}
	}

	tcpAddr, err := net.ResolveTCPAddr("tcp", *address)
	checkError(err)
	listener, err := net.ListenTCP("tcp", tcpAddr)
	checkError(err)
	for {
		conn, err := listener.Accept()
		if err != nil {
			// error handling

		} else {
			go handleConnection(conn)
		}

	}
}

func raiseUlimit() {

	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		log.Fatal("Failed to get open file limit")
	}

	log.Println("Initial open file limit ", rLimit.Max, "/", rLimit.Cur)

	if rLimit.Max < *maxConnections {
		rLimit.Max = *maxConnections
	}

	if rLimit.Cur < *maxConnections {
		rLimit.Cur = *maxConnections
	}

	err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit)

	if err != nil {
		log.Fatal("Failed to raise connection limit to ", maxConnections)
	}

	err = syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		log.Fatal("Failed to get open file limit")
	}

	log.Println("New open file limit ", rLimit.Max, "/", rLimit.Cur)

}

func Verbose(v ...interface{}) {
	if *verbose {
		log.Println(v)
	}
}

func (redis RedisInfo) Stop() {
	timeout := make(chan bool)
	done := make(chan bool)

	redis.Cmd.Process.Signal(syscall.SIGTERM)

	go func() {
		time.Sleep(10 * time.Second)
		timeout <- true
	}()

	go func() {
		redis.Cmd.Wait()
		done <- true
	}()

	select {
	case <-done:
		log.Println("Redis ", redis.Name, "terminated gracefully")
	case <-timeout:
		redis.Cmd.Process.Signal(syscall.SIGKILL)
		log.Println("Redis ", redis.Name, "termination forced")
		<-done
	}
}

func watchRunningRedis(redis *RedisInfo) {
	redis.Cmd.Wait()
	log.Println("Redis Stopped ", redis.Name)
	// TODO recover
}

func startupRedis(name string) {

	redises.Lock()
	defer redises.Unlock()

	_, alreadyStarted := redises.m[name]
	if alreadyStarted {
		return
	}

	log.Println("Starting up:", name)
	os.MkdirAll("dbs/"+name, 0770)

	cmd := exec.Command("bin/redis-server",
		"config/redis.conf",
		"--unixsocket", "redis.sock",
		"--dir", "./dbs/"+name+"/",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Start()

	info := &RedisInfo{
		Pid:  cmd.Process.Pid,
		Name: name,
		Cmd:  cmd,
	}

	go watchRunningRedis(info)

	redises.m[info.Name] = info
}

func parse(buffer []byte, length int) (command RedisCommand, err error) {
	command = RedisCommand{}

	const (
		START = iota
		PARAMS
		LENGTH
		DATA
	)

	// TODO add robust error handling

	//log.Println(string(buffer[:length]))

	state := START
	first := -1
	paramLength := -1
	pos := 0

	for i := 0; i < length; i++ {
		char := buffer[i]
		if char == '*' && state == START {
			first = i + 1
			state = PARAMS
		} else if state == PARAMS && char == '\r' {
			val := string(buffer[first:i])
			parsed, _ := strconv.Atoi(val)
			command.ParamCount = parsed
			command.Params = make([]string, parsed)
			state = START
		} else if char == '$' && state == START {
			first = i + 1
			state = LENGTH
		} else if state == LENGTH && char == '\r' {
			val := string(buffer[first:i])
			paramLength, _ = strconv.Atoi(val)
			state = DATA
		} else if state == DATA && char == '\n' {
			to := i + paramLength + 1
			if to > length {
				// NOT SUPPORTED ATM
				err = fmt.Errorf("NOT SUPPORTING LARGE COMMANDS YET")
				return
			}

			if pos >= len(command.Params) {
				err = fmt.Errorf("CORRUPT STREAM, NOT PARSING")
				return
			}
			command.Params[pos] = string(buffer[i+1 : to])
			pos++
			state = START
			i += paramLength + 2
		}

	}

	return
}

func glue(client net.Conn, server net.Conn, done chan bool) {

	var buffer []byte = make([]byte, 256)
	for {
		read, err := client.Read(buffer)
		if err != nil {
			Verbose("Connection closed")
			done <- true
			return
		}
		if read > 0 {
			server.Write(buffer[:read])
		}
	}
}

func proxyRedis(client net.Conn, name string) {
	defer client.Close()

	Verbose("Proxying :", name)

	for retries := 0; retries < 30; retries++ {
		server, err := net.Dial("unix", "dbs/"+name+"/redis.sock")
		if err != nil {
			log.Println("ERROR: connection to "+name+" not successful: ", err, " attempts: ", retries)
		} else {
			defer server.Close()
			done := make(chan bool)
			go glue(client, server, done)
			go glue(server, client, done)

			<-done
			return
		}
		time.Sleep(1 * time.Second)
	}
	log.Println("FAIL: connection to " + name + " not successful timed out")
}

func sendSimpleReply(conn net.Conn, reply string) {
	conn.Write([]byte("*1\r\n$" + strconv.Itoa(len(reply)) + "\r\n" + reply + "\r\n"))
}

func handleConnection(conn net.Conn) {

	var buffer []byte = make([]byte, 256)
	for {
		read, err := conn.Read(buffer)
		if err != nil {
			log.Println("Error handling connection:", err)
			return
		}

		parsed, err := parse(buffer, read)

		if err == nil && parsed.ParamCount == 2 && strings.ToUpper(parsed.Params[0]) == "AUTH" {
			name := parsed.Params[1]

			redises.RLock()
			_, running := redises.m[name]
			redises.RUnlock()
			if !running {
				startupRedis(parsed.Params[1])
			}
			sendSimpleReply(conn, "OK")
			go proxyRedis(conn, parsed.Params[1])
			return
		}

		sendSimpleReply(conn, "-Unrecognized Command")
	}
}

func checkError(err error) {
	if err != nil {
		log.Fatal("Fatal error ", err.Error())
	}
}

func registerExitHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
	go func() {
		<-c
		onExit()
		os.Exit(1)
	}()
}

func onExit() {
	// TODO Issue is we need to to stop signal propergation here to
	//  terminat redis gracefull on sigint, cause redis out fo the box
	//  will simply terminate
	// At least this gives redis enough time to terminate if parent gets a SIGTERM
	redises.RLock()
	for _, v := range redises.m {
		v.Stop()
	}
	redises.RUnlock()

	if *profile != "" {
		pprof.StopCPUProfile()
	}
	log.Println("Exiting...")
}
