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
	var port string = "14379"
	var url string = fmt.Sprintf("http://127.0.0.1:%s", port)
	var err error
	var containerID string
	var j *db.Job

	BeforeAll(func(ctx SpecContext) {
		// Dockerコンテナを起動
		url = fmt.Sprintf("http://127.0.0.1:%s", port)
		cmd := exec.Command("docker", "run", "-d", "--rm", "-p", fmt.Sprintf("%s:2379", port), "ghcr.io/takara9/etcd:3.6.5")
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
	}, NodeTimeout(20*time.Second))

	Describe("ジョブ機能テスト", func() {
		Context("データベースへの接続", func() {
			It("ETCDと接続", func() {
				j, err = db.NewJobController(url, "")
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("ジョブ制御", func() {

			var entryJobId string
			It("ジョブ1 登録", func() {
				jobId, err := j.EntryJob("test-job1", "sleep", "2")
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(jobId)
				entryJobId = jobId
				time.Sleep(1 * time.Second)
			})

			It("全ジョブリストからの取り出し", func() {
				jobs, err := j.GetAllJobs()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(jobs)).To(BeNumerically(">", 0))
				fmt.Printf("%-5s  %-12s  %-10s  %-20s  %-s\n", "ID", "JOB-NAME", "STATUS", "REQUESTED-TIME", "COMMAND")
				for _, job := range jobs {
					fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
					if job.Id == entryJobId {
						Expect(*job.Metadata.Name).To(Equal("test-job1"))
						Expect(db.JobStatus[job.Status.StatusCode]).To(Equal("PENDING"))
						Expect(*job.Spec.Command).To(Equal([]string{"sleep", "2"}))
					}
				}
			})

			It("ジョブ1の取り出しと実行-1", func() {
				job, err := j.FetchJob()
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
				err = j.RunJob(job)
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("終了 Job ID: %5s\n", job.Id)
			})

			It("全ジョブリストからの取り出し", func() {
				jobs, err := j.GetAllJobs()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(jobs)).To(BeNumerically(">", 0))
				fmt.Printf("%-5s  %-12s  %-10s  %-20s  %-s\n", "ID", "JOB-NAME", "STATUS", "REQUESTED-TIME", "COMMAND")
				for _, job := range jobs {
					fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
					if job.Id == entryJobId {
						Expect(*job.Metadata.Name).To(Equal("test-job1"))
						Expect(db.JobStatus[job.Status.StatusCode]).To(Equal("SUCCEEDED"))
						Expect(*job.Spec.Command).To(Equal([]string{"sleep", "2"}))
					}
				}
			})

			It("ジョブ2  失敗コマンド", func() {
				jobId, err := j.EntryJob("test-job2", "false")
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(jobId)
				entryJobId = jobId
				time.Sleep(1 * time.Second)
			})

			It("全ジョブリストからの取り出し", func() {
				jobs, err := j.GetAllJobs()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(jobs)).To(BeNumerically(">", 0))
				fmt.Printf("%-5s  %-12s  %-10s  %-20s  %-s\n", "ID", "JOB-NAME", "STATUS", "REQUESTED-TIME", "COMMAND")
				for _, job := range jobs {
					fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
					if job.Id == entryJobId {
						Expect(*job.Metadata.Name).To(Equal("test-job2"))
						Expect(db.JobStatus[job.Status.StatusCode]).To(Equal("PENDING"))
						Expect(*job.Spec.Command).To(Equal([]string{"false"}))
					}
				}
			})

			It("ジョブ2の取り出しと実行", func() {
				job, err := j.FetchJob()
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
				err = j.RunJob(job)
				// ジョブが開始されれば、ジョブが失敗してもエラーにはならない
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("終了 Job ID: %5s\n", job.Id)
			})

			It("全ジョブリストからの取り出し", func() {
				jobs, err := j.GetAllJobs()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(jobs)).To(BeNumerically(">", 0))
				fmt.Printf("%-5s  %-12s  %-10s  %-20s  %-s\n", "ID", "JOB-NAME", "STATUS", "REQUESTED-TIME", "COMMAND")
				for _, job := range jobs {
					fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
					if job.Id == entryJobId {
						Expect(*job.Metadata.Name).To(Equal("test-job2"))
						Expect(db.JobStatus[job.Status.StatusCode]).To(Equal("FAILED"))
						Expect(*job.Spec.Command).To(Equal([]string{"false"}))
					}
				}
			})

			It("ジョブ3 登録", func() {
				jobId, err := j.EntryJob("test-job3", "echo", "Hello,", " World!")
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(jobId)
				entryJobId = jobId
				time.Sleep(1 * time.Second)
			})

			It("ジョブ4 登録", func() {
				jobId, err := j.EntryJob("test-job4", "false")
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(jobId)
				time.Sleep(1 * time.Second)
			})

			It("全ジョブリストからの取り出し", func() {
				jobs, err := j.GetAllJobs()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(jobs)).To(BeNumerically(">", 0))
				fmt.Printf("%-5s  %-12s  %-10s  %-20s  %-s\n", "ID", "JOB-NAME", "STATUS", "REQUESTED-TIME", "COMMAND")
				for _, job := range jobs {
					fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
				}
			})

			It("ジョブ3の取り出しと実行", func() {
				job, err := j.FetchJob()
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
				Expect(job.Id).To(Equal(entryJobId))
				err = j.RunJob(job)
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("終了 Job ID: %5s\n", job.Id)
			})

			It("ジョブ4の取り出しと実行", func() {
				job, err := j.FetchJob()
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
				err = j.RunJob(job)
				// ジョブが開始されれば、ジョブが失敗してもエラーにはならない
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("終了 Job ID: %5s\n", job.Id)
			})

			It("全ジョブリストからの取り出し", func() {
				jobs, err := j.GetAllJobs()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(jobs)).To(BeNumerically(">", 0))
				fmt.Printf("%-5s  %-12s  %-10s  %-20s  %-s\n", "ID", "JOB-NAME", "STATUS", "REQUESTED-TIME", "COMMAND")
				for _, job := range jobs {
					fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
				}
			})

			It("ジョブ5 登録", func() {
				jobId, err := j.EntryJob("test-job5", "echo", "Hello,", " World!")
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Println(jobId)
				entryJobId = jobId
				time.Sleep(1 * time.Second)
			})

			It("全ジョブリストからの取り出し", func() {
				jobs, err := j.GetAllJobs()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(jobs)).To(BeNumerically(">", 0))
				fmt.Printf("%-5s  %-12s  %-10s  %-20s  %-s\n", "ID", "JOB-NAME", "STATUS", "REQUESTED-TIME", "COMMAND")
				for _, job := range jobs {
					fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
				}
			})

			It("未実行ジョブリストから、最も古いジョブをキャンセル", func() {
				job, err := j.FetchJob()
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("Job ID: %5s, Task: %10s, Args: %v, ReqTime: %20s\n", job.Id, *job.Metadata.Name, *job.Spec.Command, job.Spec.RequestTime.String())
				err = j.CancelJob(job.Id)
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("Cancelled Job ID: %5s\n", job.Id)
			})

			It("全ジョブリストからの取り出し", func() {
				jobs, err := j.GetAllJobs()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(jobs)).To(BeNumerically(">", 0))
				fmt.Printf("%-5s  %-12s  %-10s  %-20s  %-s\n", "ID", "JOB-NAME", "STATUS", "REQUESTED-TIME", "COMMAND")
				for _, job := range jobs {
					fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
				}
			})

			It("ジョブリストから取り出し、キャンセルしたジョブが含まれいないこと", func() {
				jobs, err := j.GetJobs(db.JOB_PENDING)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(jobs)).To(Equal(0))
				fmt.Printf("%-5s  %-12s  %-10s  %-20s  %-s\n", "ID", "JOB-NAME", "STATUS", "REQUESTED-TIME", "COMMAND")
				for _, job := range jobs {
					fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
				}
			})

			It("古いジョブ記録の消去", func() {
				err := j.CleanupJob(3)
				Expect(err).NotTo(HaveOccurred())
				jobs, err := j.GetAllJobs()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(jobs)).To(BeNumerically(">", 0))
				fmt.Printf("%-5s  %-12s  %-10s  %-20s  %-s\n", "ID", "JOB-NAME", "STATUS", "REQUESTED-TIME", "COMMAND")
				for _, job := range jobs {
					fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
				}
			})

			It("ジョブ6 30秒時間継続するシェル 登録  ", func() {
				jobId, err := j.EntryJob("test-job6", "testdata/test-job1.sh")
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(jobId)
				time.Sleep(1 * time.Second)
			})

			It("全ジョブリストからの取り出し", func() {
				jobs, err := j.GetAllJobs()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(jobs)).To(BeNumerically(">", 0))
				fmt.Printf("%-5s  %-12s  %-10s  %-20s  %-s\n", "ID", "JOB-NAME", "STATUS", "REQUESTED-TIME", "COMMAND")
				for _, job := range jobs {
					fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
				}
			})

			It("未実行のジョブ６の取り出し", func() {
				jobs, err := j.GetJobs(db.JOB_PENDING)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(jobs)).To(BeNumerically(">", 0))
				fmt.Printf("%-5s  %-12s  %-10s  %-20s  %-s\n", "ID", "JOB-NAME", "STATUS", "REQUESTED-TIME", "COMMAND")
				for _, job := range jobs {
					fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
				}
			})

			It("未実行のジョブ６の実行開始", func() {
				job, err := j.FetchJob()
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)

				go func() {
					err = j.RunJob(job)
					Expect(err).NotTo(HaveOccurred())
					fmt.Printf("Completed Job ID: %5s\n", job.Id)
				}()
			})

			It("全ジョブリストからの取り出し", func() {
				jobs, err := j.GetAllJobs()
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("%-5s  %-12s  %-10s  %-20s  %-s\n", "ID", "JOB-NAME", "STATUS", "DURATION-TIME", "COMMAND")
				for _, job := range jobs {
					if job.Spec.StartTime != nil {
						fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], (time.Since(*job.Spec.StartTime) * time.Second), *job.Spec.Command)
					} else {
						fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], 0*time.Second, *job.Spec.Command)
					}
				}
			})

			It("実行中シェル６の監視", func() {
				for {
					jobs, err := j.GetJobs(db.JOB_RUNNING)
					Expect(err).NotTo(HaveOccurred())
					fmt.Println("Running Jobs Count:", len(jobs))
					if len(jobs) == 0 {
						break
					}
					for _, job := range jobs {
						fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
					}
					time.Sleep(5 * time.Second)
				}
			})

			It("全ジョブリストからの取り出し", func() {
				jobs, err := j.GetAllJobs()
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("%-5s  %-12s  %-10s  %-20s  %-s\n", "ID", "JOB-NAME", "STATUS", "REQUESTED-TIME", "COMMAND")
				for _, job := range jobs {
					fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
				}
			})

			It("ジョブ７（失敗シェル)の登録", func() {
				jobId, err := j.EntryJob("test-job5", "testdata/test-job2.sh")
				Expect(err).NotTo(HaveOccurred())
				fmt.Println(jobId)
				time.Sleep(1 * time.Second)
			})

			It("全ジョブリストからの取り出し", func() {
				jobs, err := j.GetAllJobs()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(jobs)).To(BeNumerically(">", 0))
				fmt.Printf("%-5s  %-12s  %-10s  %-20s  %-s\n", "ID", "JOB-NAME", "STATUS", "REQUESTED-TIME", "COMMAND")
				for _, job := range jobs {
					fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
				}
			})

			It("ジョブ７の実行", func() {
				job, err := j.FetchJob()
				Expect(err).NotTo(HaveOccurred())
				//fmt.Printf("Job ID: %5s, Task: %10s, Args: %v, ReqTime: %20s\n", job.Id, *job.Metadata.Name, *job.Spec.Command, job.Spec.RequestTime.String())
				fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
				go func() {
					err = j.RunJob(job)
					Expect(err).NotTo(HaveOccurred())
					fmt.Printf("Completed Job ID: %5s\n", job.Id)
				}()
				time.Sleep(2 * time.Second)
			})

			It("実行中ジョブありで、ジョブリストの取り出し", func() {
				jobs, err := j.GetAllJobs()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(jobs)).To(BeNumerically(">", 0))
				fmt.Printf("%-5s  %-12s  %-10s  %-20s  %-s\n", "ID", "JOB-NAME", "STATUS", "REQUESTED-TIME", "COMMAND")
				for _, job := range jobs {
					fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
				}
			})

			// Eventually に変更する
			It("ジョブの終了待ち", func() {
				for {
					jobs, err := j.GetJobs(db.JOB_RUNNING)
					Expect(err).NotTo(HaveOccurred())
					fmt.Println("Running Jobs Count:", len(jobs))
					if len(jobs) == 0 {
						break
					}
					for _, job := range jobs {
						fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
					}
					time.Sleep(5 * time.Second)
				}
			})

			It("終了後のジョブリストの取り出し", func() {
				jobs, err := j.GetAllJobs()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(jobs)).To(BeNumerically(">", 0))
				fmt.Printf("%-5s  %-12s  %-10s  %-20s  %-s\n", "ID", "JOB-NAME", "STATUS", "REQUESTED-TIME", "COMMAND")
				for _, job := range jobs {
					fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
				}
			})

			It("古いジョブ記録の消去", func() {
				err := j.CleanupJob(1)
				Expect(err).NotTo(HaveOccurred())
				jobs, err := j.GetAllJobs()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(jobs)).To(BeNumerically(">", 0))
				fmt.Printf("%-5s  %-12s  %-10s  %-20s  %-s\n", "ID", "JOB-NAME", "STATUS", "REQUESTED-TIME", "COMMAND")
				for _, job := range jobs {
					fmt.Printf("%-5s  %-12s  %-10s  %-20s %-v\n", job.Id, *job.Metadata.Name, db.JobStatus[job.Status.StatusCode], job.Spec.RequestTime.Format(time.DateTime), *job.Spec.Command)
				}
			})

		})
	})
})
