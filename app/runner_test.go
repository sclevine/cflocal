package app_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"strings"

	docker "github.com/docker/docker/client"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	. "github.com/sclevine/cflocal/app"
	"github.com/sclevine/cflocal/utils"
)

var _ = Describe("Runner", func() {
	var (
		runner   *Runner
		client   *docker.Client
		logs     *gbytes.Buffer
		exitChan chan struct{}
	)

	BeforeEach(func() {
		var err error
		client, err = docker.NewEnvClient()
		Expect(err).NotTo(HaveOccurred())
		logs = gbytes.NewBuffer()
		exitChan = make(chan struct{})
		runner = &Runner{
			Docker:   client,
			Logs:     io.MultiWriter(logs, GinkgoWriter),
			ExitChan: exitChan,
		}
	})

	Describe("#Run", func() {
		It("should run the provided droplet with the provided launcher", func() {
			stager := &Stager{
				DiegoVersion: "0.1482.0",
				GoVersion:    "1.7",
				Docker:       client,
				Logs:         GinkgoWriter,
			}
			appFileContents := bytes.NewBufferString("some-contents")
			appTar, err := utils.TarFile("some-file", appFileContents, int64(appFileContents.Len()), 0644)
			Expect(err).NotTo(HaveOccurred())
			colorize := func(text string) string { return text + " %" }
			droplet, dropletSize, err := stager.Stage("some-app", colorize, appTar, []string{
				"https://github.com/sclevine/cflocal-buildpack#v0.0.1",
			})
			Expect(err).NotTo(HaveOccurred())
			defer droplet.Close()

			launcher, launcherSize, err := stager.Launcher()
			Expect(err).NotTo(HaveOccurred())
			defer launcher.Close()

			port := freePort()

			go func() {
				defer GinkgoRecover()
				defer close(exitChan)
				Expect(get(fmt.Sprintf("http://localhost:%d/", port))).To(Equal(runningEnvFixture))
			}()

			config := &RunConfig{
				Droplet:      droplet,
				DropletSize:  dropletSize,
				Launcher:     launcher,
				LauncherSize: launcherSize,
				Port:         port,
			}
			status, err := runner.Run("some-app", colorize, config)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(Equal(137))

			Expect(logs.Contents()).To(MatchRegexp(`\[some-app\] % \S+ Log message from stdout.`))
			Expect(logs.Contents()).To(MatchRegexp(`\[some-app\] % \S+ Log message from stderr.`))

			Eventually(exitChan, "5s").Should(BeClosed())

			// test that no "some-app-staging-GUID" containers exist
		})

		Context("on failure", func() {
			// test failure cases using reverse proxy
		})
	})
})

func get(url string) string {
	var body io.ReadCloser
	EventuallyWithOffset(1, func() error {
		response, err := http.Get(url)
		if err != nil {
			return err
		}
		body = response.Body
		return nil
	}).Should(Succeed())
	defer body.Close()
	bodyBytes, err := ioutil.ReadAll(body)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return string(bodyBytes)
}

func freePort() uint {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	defer listener.Close()

	address := listener.Addr().String()
	portStr := strings.SplitN(address, ":", 2)[1]
	port, err := strconv.ParseUint(portStr, 10, 32)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return uint(port)
}
