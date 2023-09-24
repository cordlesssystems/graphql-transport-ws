package main

import (
	"flag"
	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/cordlesssystems/graphql-transport-ws/contrib/gqlgen"
	"github.com/cordlesssystems/graphql-transport-ws/contrib/gqlgen/example/graph"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime/pprof"
	"runtime/trace"
)

var (
	cpuProfile       bool
	memProfile       bool
	goroutineProfile bool
	traceProfile     bool
)

func init() {
	flag.BoolVar(&cpuProfile, "profcpu", false, "Enable CPU profile")
	flag.BoolVar(&memProfile, "profmem", false, "Enable Mem profile")
	flag.BoolVar(&goroutineProfile, "profgo", false, "Enable goroutine profile")
	flag.BoolVar(&traceProfile, "proftrace", false, "Enable execution tracing")

}

const defaultPort = "8080"

func NewDefaultServer(es graphql.ExecutableSchema) *handler.Server {
	srv := handler.New(es)

	srv.AddTransport(&gqlgen.Transport{})
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{})

	srv.SetQueryCache(lru.New(1000))

	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New(100),
	})

	return srv
}

func startProfilers() {

	if cpuProfile {
		f, err := os.Create("cpu.prof")
		if err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer f.Close()
		if err = pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}
	if traceProfile {
		f, err := os.Create("trace.prof")
		if err != nil {
			log.Fatal("could not start execution trace: ", err)
		}
		defer f.Close()
		if err = trace.Start(f); err != nil {
			log.Fatal("could not start execution trace: ", err)
		}
		defer trace.Stop()
	}

}

func main() {

	flag.Parse()

	startProfilers()

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	srv := NewDefaultServer(graph.NewExecutableSchema(graph.Config{Resolvers: &graph.Resolver{}}))

	http.Handle("/", playground.Handler("GraphQL playground", "/query"))
	http.Handle("/query", srv)

	log.Printf("connect to http://localhost:%s/ for GraphQL playground", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
