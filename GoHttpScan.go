package main

import (
	"bufio"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fatih/color"
)

var outputMutex sync.Mutex
var quitChan = make(chan os.Signal, 1)

func main() {
	hello := `
 ██████╗  ██████╗ ███████╗ ██████╗ █████╗ ███╗   ██╗
██╔════╝ ██╔═══██╗██╔════╝██╔════╝██╔══██╗████╗  ██║
██║  ███╗██║   ██║███████╗██║     ███████║██╔██╗ ██║
██║   ██║██║   ██║╚════██║██║     ██╔══██║██║╚██╗██║
╚██████╔╝╚██████╔╝███████║╚██████╗██║  ██║██║ ╚████║
 ╚═════╝  ╚═════╝ ╚══════╝ ╚═════╝╚═╝  ╚═╝╚═╝  ╚═══╝                                                
	`
	fmt.Println(hello)

	inputFile := flag.String("l", "", "URL list file")
	outputFile := flag.String("o", "output.txt", "Output file")
	threads := flag.Int("t", 10, "Number of threads (default 10)")
	showHelp := flag.Bool("h", false, "Show help")

	flag.Parse()

	if *showHelp || *inputFile == "" {
		fmt.Println("Usage: GoHttpScan -l <urls.txt> -o <output.txt> [-t <threads>] [-h]")
		fmt.Println(" -l : URL list file")
		fmt.Println(" -o : Output file (default output.txt)")
		fmt.Println(" -t : Number of threads (default 10)")
		fmt.Println(" -h : Show help")
		return
	}

	file, err := os.Open(*inputFile)
	if err != nil {
		fmt.Println("Failed to open file:", err)
		return
	}
	defer file.Close()

	outFile, err := os.Create(*outputFile)
	if err != nil {
		fmt.Println("Failed to create output file:", err)
		return
	}
	defer outFile.Close()

	var wg sync.WaitGroup
	scanner := bufio.NewScanner(file)
	urls := make(chan string)

	guard := make(chan struct{}, *threads)

	for i := 0; i < *threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for url := range urls {
				guard <- struct{}{}
				checkURL(url, outFile)
				<-guard
			}
		}()
	}

	go handleInterrupt()

	go func() {
		for scanner.Scan() {
			select {
			case urls <- scanner.Text():
			case <-quitChan:
				close(urls)
				return
			}
		}
		close(urls)
	}()

	wg.Wait()

	if err := scanner.Err(); err != nil {
		fmt.Println("Error reading file:", err)
	}
}

func handleInterrupt() {
	signal.Notify(quitChan, os.Interrupt, syscall.SIGTERM)
	<-quitChan
	outputMutex.Lock()
	fmt.Println("Exit")
	outputMutex.Unlock()
	close(quitChan)
	os.Exit(0)
}

func checkURL(rawURL string, outputFile *os.File) {
	client := http.Client{
		Timeout: 10 * time.Second,
	}

	urlsToCheck := []string{rawURL}

	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		urlsToCheck = []string{"http://" + rawURL, "https://" + rawURL}
	}

	for _, u := range urlsToCheck {
		parsedURL, err := url.ParseRequestURI(u)
		if err != nil {
			outputMutex.Lock()
			color.Yellow("Malformed URL: %s", u)
			outputMutex.Unlock()
			return
		}

		resp, err := client.Get(parsedURL.String())
		if err != nil {
			outputMutex.Lock()
			color.Red("Cannot access %s", u)
			outputMutex.Unlock()
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			outputMutex.Lock()
			color.Green("URL: %s Status Code: %d", u, resp.StatusCode)
			_, err := outputFile.WriteString(fmt.Sprintf("%s\n", u))
			if err != nil {
				fmt.Println("Failed to write to output file:", err)
			}
			outputMutex.Unlock()
		} else {
			outputMutex.Lock()
			color.Red("URL: %s Status Code: %d", u, resp.StatusCode)
			outputMutex.Unlock()
		}
	}
}
