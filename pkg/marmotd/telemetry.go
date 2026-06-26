package marmotd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

type Telemetry struct {
	meterProvider *sdkmetric.MeterProvider
}

type telemetrySnapshot struct {
	vmTotal              int64
	vmStatusRunning      int64
	vmStatusError        int64
	vmStatusPending      int64
	virtualNetworkTotal  int64
	virtualNetworkActive int64
	virtualNetworkError  int64
	volumeTotal          int64
	volumeAvailable      int64
	volumeFailed         int64
	totalCPU             int64
	allocatedCPU         int64
	freeCPU              int64
	totalMemoryMB        int64
	allocatedMemoryMB    int64
}

func RegisterOpenTelemetryMetrics(e *echo.Echo, database *db.Database) (*Telemetry, error) {
	if e == nil {
		return nil, fmt.Errorf("echo instance is nil")
	}
	if database == nil {
		return nil, fmt.Errorf("database is nil")
	}

	exporter, err := prometheus.New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize prometheus exporter: %w", err)
	}

	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	otel.SetMeterProvider(meterProvider)

	meter := meterProvider.Meter("github.com/takara9/marmot/marmotd")

	vmTotal, err := meter.Int64ObservableGauge("marmot_vm_total", metric.WithDescription("Total number of virtual machines"))
	if err != nil {
		return nil, err
	}
	vmStatusRunning, err := meter.Int64ObservableGauge("marmot_vm_status_running", metric.WithDescription("Number of VMs in RUNNING status"))
	if err != nil {
		return nil, err
	}
	vmStatusError, err := meter.Int64ObservableGauge("marmot_vm_status_error", metric.WithDescription("Number of VMs in ERROR status"))
	if err != nil {
		return nil, err
	}
	vmStatusPending, err := meter.Int64ObservableGauge("marmot_vm_status_pending", metric.WithDescription("Number of VMs in PENDING status"))
	if err != nil {
		return nil, err
	}

	virtualNetworkTotal, err := meter.Int64ObservableGauge("marmot_virtual_network_total", metric.WithDescription("Total number of virtual networks"))
	if err != nil {
		return nil, err
	}
	virtualNetworkActive, err := meter.Int64ObservableGauge("marmot_virtual_network_active", metric.WithDescription("Number of virtual networks in ACTIVE status"))
	if err != nil {
		return nil, err
	}
	virtualNetworkError, err := meter.Int64ObservableGauge("marmot_virtual_network_error", metric.WithDescription("Number of virtual networks in ERROR status"))
	if err != nil {
		return nil, err
	}

	volumeTotal, err := meter.Int64ObservableGauge("marmot_volume_total", metric.WithDescription("Total number of volumes"))
	if err != nil {
		return nil, err
	}
	volumeAvailable, err := meter.Int64ObservableGauge("marmot_volume_available", metric.WithDescription("Number of volumes in AVAILABLE status"))
	if err != nil {
		return nil, err
	}
	volumeFailed, err := meter.Int64ObservableGauge("marmot_volume_failed", metric.WithDescription("Number of volumes in failed status"))
	if err != nil {
		return nil, err
	}

	totalCPU, err := meter.Int64ObservableGauge("marmot_cpu_total", metric.WithDescription("Total CPU cores across hosts"))
	if err != nil {
		return nil, err
	}
	allocatedCPU, err := meter.Int64ObservableGauge("marmot_cpu_allocated", metric.WithDescription("Allocated CPU cores across hosts"))
	if err != nil {
		return nil, err
	}
	freeCPU, err := meter.Int64ObservableGauge("marmot_cpu_free", metric.WithDescription("Free CPU cores across hosts"))
	if err != nil {
		return nil, err
	}

	totalMemoryMB, err := meter.Int64ObservableGauge("marmot_memory_total_mb", metric.WithDescription("Total memory in MB across hosts"))
	if err != nil {
		return nil, err
	}
	allocatedMemoryMB, err := meter.Int64ObservableGauge("marmot_memory_allocated_mb", metric.WithDescription("Allocated memory in MB across hosts"))
	if err != nil {
		return nil, err
	}

	_, err = meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		snapshot, collectErr := collectTelemetrySnapshot(database)
		if collectErr != nil {
			slog.Warn("failed to collect telemetry snapshot", "err", collectErr)
			return nil
		}

		observer.ObserveInt64(vmTotal, snapshot.vmTotal)
		observer.ObserveInt64(vmStatusRunning, snapshot.vmStatusRunning)
		observer.ObserveInt64(vmStatusError, snapshot.vmStatusError)
		observer.ObserveInt64(vmStatusPending, snapshot.vmStatusPending)
		observer.ObserveInt64(virtualNetworkTotal, snapshot.virtualNetworkTotal)
		observer.ObserveInt64(virtualNetworkActive, snapshot.virtualNetworkActive)
		observer.ObserveInt64(virtualNetworkError, snapshot.virtualNetworkError)
		observer.ObserveInt64(volumeTotal, snapshot.volumeTotal)
		observer.ObserveInt64(volumeAvailable, snapshot.volumeAvailable)
		observer.ObserveInt64(volumeFailed, snapshot.volumeFailed)
		observer.ObserveInt64(totalCPU, snapshot.totalCPU)
		observer.ObserveInt64(allocatedCPU, snapshot.allocatedCPU)
		observer.ObserveInt64(freeCPU, snapshot.freeCPU)
		observer.ObserveInt64(totalMemoryMB, snapshot.totalMemoryMB)
		observer.ObserveInt64(allocatedMemoryMB, snapshot.allocatedMemoryMB)

		return nil
	},
		vmTotal,
		vmStatusRunning,
		vmStatusError,
		vmStatusPending,
		virtualNetworkTotal,
		virtualNetworkActive,
		virtualNetworkError,
		volumeTotal,
		volumeAvailable,
		volumeFailed,
		totalCPU,
		allocatedCPU,
		freeCPU,
		totalMemoryMB,
		allocatedMemoryMB,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register metric callback: %w", err)
	}

	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))

	return &Telemetry{meterProvider: meterProvider}, nil
}

func (t *Telemetry) Shutdown(ctx context.Context) error {
	if t == nil || t.meterProvider == nil {
		return nil
	}
	return t.meterProvider.Shutdown(ctx)
}

func collectTelemetrySnapshot(database *db.Database) (telemetrySnapshot, error) {
	snapshot := telemetrySnapshot{}

	servers, err := database.GetServers()
	if err != nil {
		return telemetrySnapshot{}, err
	}
	for _, server := range servers {
		snapshot.vmTotal++
		if server.Status == nil {
			continue
		}
		switch server.Status.StatusCode {
		case db.SERVER_RUNNING:
			snapshot.vmStatusRunning++
		case db.SERVER_ERROR:
			snapshot.vmStatusError++
		case db.SERVER_PENDING:
			snapshot.vmStatusPending++
		}
	}

	networks, err := database.GetVirtualNetworks()
	if err != nil {
		return telemetrySnapshot{}, err
	}
	snapshot.virtualNetworkTotal = int64(len(networks))
	for _, network := range networks {
		if network.Status == nil {
			continue
		}
		switch network.Status.StatusCode {
		case db.NETWORK_ACTIVE:
			snapshot.virtualNetworkActive++
		case db.NETWORK_ERROR:
			snapshot.virtualNetworkError++
		}
	}

	volumes, err := database.GetVolumes()
	if err != nil {
		return telemetrySnapshot{}, err
	}
	snapshot.volumeTotal = int64(len(volumes))
	for _, volume := range volumes {
		if volume.Status == nil {
			continue
		}
		switch volume.Status.StatusCode {
		case db.VOLUME_AVAILABLE:
			snapshot.volumeAvailable++
		case db.VOLUME_ERROR, db.VOLUME_UNAVAILABLE:
			snapshot.volumeFailed++
		}
	}

	hostStatuses, err := database.GetAllHostStatus()
	if err != nil {
		return telemetrySnapshot{}, err
	}
	for _, status := range hostStatuses {
		accumulateHostCapacityAndAllocation(&snapshot, status)
	}

	snapshot.freeCPU = snapshot.totalCPU - snapshot.allocatedCPU
	if snapshot.freeCPU < 0 {
		snapshot.freeCPU = 0
	}

	return snapshot, nil
}

func accumulateHostCapacityAndAllocation(snapshot *telemetrySnapshot, status api.HostStatus) {
	if status.Capacity != nil {
		if status.Capacity.CpuCores != nil {
			snapshot.totalCPU += int64(*status.Capacity.CpuCores)
		}
		if status.Capacity.MemoryMB != nil {
			snapshot.totalMemoryMB += int64(*status.Capacity.MemoryMB)
		}
	}

	if status.Allocation != nil {
		if status.Allocation.AllocatedCpuCores != nil {
			snapshot.allocatedCPU += int64(*status.Allocation.AllocatedCpuCores)
		}
		if status.Allocation.AllocatedMemoryMB != nil {
			snapshot.allocatedMemoryMB += int64(*status.Allocation.AllocatedMemoryMB)
		}
	}
}
