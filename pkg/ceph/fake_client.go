package ceph

import "context"

type FakeClient struct {
	CreateVolumeFunc func(ctx context.Context, req VolumeRequest) error
	DeleteVolumeFunc func(ctx context.Context, pool, image string) error
	StatVolumeFunc   func(ctx context.Context, pool, image string) (VolumeInfo, error)
	ListVolumesFunc  func(ctx context.Context, pool string) ([]string, error)

	Created []VolumeRequest
	Deleted []string
	Listed  []string
	Stated  []string
}

func (f *FakeClient) CreateVolume(ctx context.Context, req VolumeRequest) error {
	f.Created = append(f.Created, req)
	if f.CreateVolumeFunc != nil {
		return f.CreateVolumeFunc(ctx, req)
	}
	return nil
}

func (f *FakeClient) DeleteVolume(ctx context.Context, pool, image string) error {
	f.Deleted = append(f.Deleted, pool+"/"+image)
	if f.DeleteVolumeFunc != nil {
		return f.DeleteVolumeFunc(ctx, pool, image)
	}
	return nil
}

func (f *FakeClient) StatVolume(ctx context.Context, pool, image string) (VolumeInfo, error) {
	f.Stated = append(f.Stated, pool+"/"+image)
	if f.StatVolumeFunc != nil {
		return f.StatVolumeFunc(ctx, pool, image)
	}
	return VolumeInfo{Pool: pool, Image: image, ProviderVolumeID: pool + "/" + image}, nil
}

func (f *FakeClient) ListVolumes(ctx context.Context, pool string) ([]string, error) {
	f.Listed = append(f.Listed, pool)
	if f.ListVolumesFunc != nil {
		return f.ListVolumesFunc(ctx, pool)
	}
	return []string{}, nil
}

func (f *FakeClient) Cleanup() error {
	return nil
}
