package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"media_agent/hertz_infra/clienthertz"
	"media_agent/hertz_infra/config"
	"media_agent/media_workflow/biz/dal"
	capabilityrepo "media_agent/media_workflow/biz/dal/repo/capability"
	jobrepo "media_agent/media_workflow/biz/dal/repo/job"
	"media_agent/media_workflow/biz/shared/client"
	"media_agent/media_workflow/biz/workflow/conductor"
	"media_agent/media_workflow/biz/workflow/worker_gateway"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	loader, err := config.NewLoader()
	if err != nil {
		return err
	}
	defer loader.Close()
	cfg := loader.Current()
	worker := cfg.GetManagedWorker()
	if worker == nil || !worker.GetEnabled() {
		log.Print("managed worker disabled")
		return nil
	}
	if cfg.GetDatabase() == nil || !cfg.GetDatabase().GetEnabled() {
		return fmt.Errorf("managed worker requires database")
	}
	db, err := dal.NewDB(cfg.GetDatabase())
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	flow := &worker_gateway.Flow{
		Dispatches: jobrepo.NewDispatchRepo(db), Jobs: jobrepo.NewJobRepo(db), StepRuns: jobrepo.NewStepRunRepo(db),
		Capabilities: capabilityrepo.NewRepo(db), Engine: &conductor.ExecutionEngine{Client: conductor.NewClient(os.Getenv("CONDUCTOR_URL"))},
	}
	instanceID := worker.GetInstanceId()
	if instanceID == "" {
		instanceID = uuid.NewString()
	}
	bindings := []worker_gateway.ManagedBinding{}
	executors := map[string]worker_gateway.ManagedExecutor{}
	clientSuite := clienthertz.NewClientSuiteFromConfig(cfg)
	for _, binding := range worker.GetBindings() {
		clientCfg := cfg.GetClient()[binding.GetExecutorClient()]
		if clientCfg == nil || !clientCfg.GetEnabled() || clientCfg.GetBaseDomain() == "" {
			return fmt.Errorf("managed executor client is not configured")
		}
		suite, err := clientSuite.NewAppClientSuite(binding.GetExecutorClient())
		if err != nil {
			return err
		}
		httpClient, err := suite.NewClient()
		if err != nil {
			return err
		}
		key := fmt.Sprintf("%s@%d", binding.GetCapabilityKey(), binding.GetCapabilityVersion())
		executors[key] = &client.ManagedExecutor{Client: httpClient, BaseURL: clientCfg.GetBaseDomain()}
		bindings = append(bindings, worker_gateway.ManagedBinding{CapabilityKey: binding.GetCapabilityKey(), CapabilityVersion: int(binding.GetCapabilityVersion()), PoolKey: binding.GetPoolKey()})
	}
	runtime, err := worker_gateway.NewManagedRuntime(ctx, flow, worker.GetWorkerId(), instanceID, bindings)
	if err != nil {
		return err
	}
	go flow.RunResultSubmitter(ctx, time.Second)
	go flow.RunLeaseReaper(ctx, 15*time.Second)
	log.Printf("managed worker starting: worker_id=%s instance_id=%s", worker.GetWorkerId(), instanceID)
	err = runtime.Run(ctx, time.Duration(worker.GetPollIntervalMs())*time.Millisecond, executors)
	if ctx.Err() != nil {
		return nil
	}
	return err
}
