package db_test

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/pkg/db"
)

var _ = Describe("Jobs", Ordered, func() {
	var url string
	var err error
	var containerID string
	var j *db.Job

	BeforeAll(func(ctx SpecContext) {
		/*
			// Setup slog
			opts := &slog.HandlerOptions{
				AddSource: true,
				Level:     slog.LevelDebug,
			}
			logger := slog.New(slog.NewJSONHandler(os.Stderr, opts))
			slog.SetDefault(logger)
		*/

		// Dockerコンテナを起動
		url = "http://127.0.0.1:5379"
		cmd := exec.Command("docker", "run", "-d", "--name", "jobEtcdDb", "-p", "5379:2379", "-p", "5380:2380", "ghcr.io/takara9/etcd:3.6.5")
		output, err := cmd.CombinedOutput()
		if err != nil {
			Fail(fmt.Sprintf("Failed to start container: %s, %v", string(output), err))
		}
		containerID = string(output[:12]) // 最初の12文字をIDとして取得
		fmt.Printf("Container started with ID: %s\n", containerID)

		time.Sleep(10 * time.Second) // コンテナが起動するまで待機
	}, NodeTimeout(20*time.Second))

	AfterAll(func(ctx SpecContext) {
		// Dockerコンテナを停止・削除
		fmt.Println("STOPPING CONTAINER:", containerID)
		cmd := exec.Command("docker", "stop", containerID)
		_, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to stop container: %v\n", err)
		}
		cmd = exec.Command("docker", "rm", containerID)
		_, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to remove container: %v\n", err)
		}
	}, NodeTimeout(20*time.Second))

	BeforeEach(func() {
	})

	AfterEach(func() {
	})

	Describe("Test etcd", func() {
		Context("Test Connection to etcd", func() {
			It("Create Job control instance", func() {
				j, err = db.NewJobController(url, "")
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("Job Control", func() {
			It("Entry new job", func() {
				jobId, err := j.EntryJob("test job1", "sleep", "2")
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(jobId)
				time.Sleep(1 * time.Second)
			})

			It("Entry new job", func() {
				jobId, err := j.EntryJob("test job2", "echo", "Hello,", " World!")
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(jobId)
				time.Sleep(1 * time.Second)
			})

			It("Entry new job", func() {
				jobId, err := j.EntryJob("test job3", "false")
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(jobId)
				time.Sleep(1 * time.Second)
			})

			It("Get Job entry list", func() {
				jobs, err := j.GetJobs(db.JOB_PENDING)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(jobs)).To(BeNumerically(">", 0))
				for _, job := range jobs {
					GinkgoWriter.Printf("Job ID: %s, Task: %s, Args: %v, ReqTime: %s\n", job.Id, job.JobName, job.Cmd, job.ReqTime.String())
				}
			})

			It("Fetch and Cancel a job", func() {
				job, err := j.FetchJob()
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("Job ID: %s, Task:[%s], Args: %v, ReqTime: %s\n", job.Id, job.JobName, job.Cmd, job.ReqTime.String())
				err = j.CancelJob(job.Id)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("Cancelled Job ID: %s\n", job.Id)
			})

			It("Get Job entry list", func() {
				jobs, err := j.GetJobs(db.JOB_PENDING)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(jobs)).To(BeNumerically(">", 0))
				for _, job := range jobs {
					GinkgoWriter.Printf("Job ID: %s, Task:[%s], Args: %v, ReqTime: %s\n", job.Id, job.JobName, job.Cmd, job.ReqTime.String())
				}
			})

			It("Fetch and Run a job #1", func() {
				job, err := j.FetchJob()
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("Job ID: %s, Task:[%s], Args: %v, ReqTime: %s\n", job.Id, job.JobName, job.Cmd, job.ReqTime.String())

				err = j.RunJob(job)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("Completed Job ID: %s\n", job.Id)
			})

			It("Get Job entry list", func() {
				jobs, err := j.GetJobs(db.JOB_PENDING)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(jobs)).To(BeNumerically(">", 0))
				for _, job := range jobs {
					GinkgoWriter.Printf("Job ID: %s, Task:[%s], Args: %v, ReqTime: %s\n", job.Id, job.JobName, job.Cmd, job.ReqTime.String())
				}
			})

			It("Fetch and Run a job #2", func() {
				job, err := j.FetchJob()
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("Job ID: %s, Task:[%s], Args: %v, ReqTime: %s\n", job.Id, job.JobName, job.Cmd, job.ReqTime.String())

				err = j.RunJob(job)
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("Completed Job ID: %s\n", job.Id)
			})

			It("Entry new job", func() {
				jobId, err := j.EntryJob("test job4", "testdata/test-job1.sh")
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(jobId)
				time.Sleep(1 * time.Second)
			})

			It("Fetch and Run a job #4", func() {
				job, err := j.FetchJob()
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("Job ID: %s, Task:[%s], Args: %v, ReqTime: %s\n", job.Id, job.JobName, job.Cmd, job.ReqTime.String())

				go func() {
					err = j.RunJob(job)
					Expect(err).NotTo(HaveOccurred())
					GinkgoWriter.Printf("Completed Job ID: %s\n", job.Id)
				}()
				time.Sleep(2 * time.Second)
			})

			It("Get Job entry list", func() {
				for {
					jobs, err := j.GetJobs(db.JOB_RUNNING)
					Expect(err).NotTo(HaveOccurred())
					fmt.Println("Running Jobs Count:", len(jobs))
					if len(jobs) == 0 {
						break
					}
					for _, job := range jobs {
						GinkgoWriter.Printf("Job ID: %s, Task:[%s], Args: %v, ReqTime: %s\n", job.Id, job.JobName, job.Cmd, job.ReqTime.String())
					}
					time.Sleep(5 * time.Second)
				}
			})

			It("Entry new job", func() {
				jobId, err := j.EntryJob("test job5", "testdata/test-job2.sh")
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(jobId)
				time.Sleep(1 * time.Second)
			})

			It("Fetch and Run a job #5", func() {
				job, err := j.FetchJob()
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("Job ID: %s, Task:[%s], Args: %v, ReqTime: %s\n", job.Id, job.JobName, job.Cmd, job.ReqTime.String())

				go func() {
					err = j.RunJob(job)
					Expect(err).NotTo(HaveOccurred())
					GinkgoWriter.Printf("Completed Job ID: %s\n", job.Id)
				}()
				time.Sleep(2 * time.Second)
			})

			It("Get Job entry list", func() {
				for {
					jobs, err := j.GetJobs(db.JOB_RUNNING)
					Expect(err).NotTo(HaveOccurred())
					fmt.Println("Running Jobs Count:", len(jobs))
					if len(jobs) == 0 {
						break
					}
					for _, job := range jobs {
						GinkgoWriter.Printf("Job ID: %s, Task:[%s], Args: %v, ReqTime: %s\n", job.Id, job.JobName, job.Cmd, job.ReqTime.String())
					}
					time.Sleep(5 * time.Second)
				}
			})
		})
	})
})
