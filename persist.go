package main

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"cloud.google.com/go/firestore"
	"k8s.io/klog/v2"
)

var (
	projectID = "yolo-checker"
)

type Blob struct {
	Results   []Result
	Timestamp time.Time
}

type Persister interface {
	Get(context.Context, string) ([]Result, error)
	Set(context.Context, string, []Result) error
}

func NewPersist(ctx context.Context, backend string) (Persister, error) {
	gob.Register(&Blob{})

	if backend == "" {
		backend = os.Getenv("PERSIST_BACKEND")
	}

	switch backend {
	case "":
		return NewNullPersister(ctx)
	case "disk":
		return NewDiskPersister(ctx)
	case "firestore":
		return NewFirePersister(ctx)
	default:
		return nil, fmt.Errorf("unknown persister: %q", backend)
	}
}

type NullPersister struct{}

func NewNullPersister(_ context.Context) (*NullPersister, error) {
	return &NullPersister{}, nil
}

func (p *NullPersister) Get(_ context.Context, _ string) ([]Result, error) {
	return nil, nil
}

func (p *NullPersister) Set(_ context.Context, _ string, _ []Result) error {
	return nil
}

type DiskPersister struct {
	path string
}

func NewDiskPersister(_ context.Context) (*DiskPersister, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("cache dir: %w", err)
	}
	d := &DiskPersister{}
	d.path = filepath.Join(root, "yoloc", "persist")

	if err := os.MkdirAll(d.path, 0o700); err != nil {
		return nil, err
	}

	return d, nil
}

func (p *DiskPersister) keyPath(key string) string {
	kr := regexp.MustCompile(`\W+`)
	key = kr.ReplaceAllString(key, "_")
	return filepath.Join(p.path, key)
}

func (p *DiskPersister) Get(ctx context.Context, key string) ([]Result, error) {
	kp := p.keyPath(key)
	klog.Infof("checking %s ...", kp)

	_, err := os.Stat(kp)
	if err != nil {
		return nil, nil
	}

	bs, err := os.ReadFile(kp)
	if err != nil {
		return nil, fmt.Errorf("readfile: %v", err)
	}

	bl := &Blob{}
	enc := gob.NewDecoder(bytes.NewReader(bs))
	err = enc.Decode(&bl)
	if err != nil {
		return nil, fmt.Errorf("decode fail: %v", err)
	}

	cutoff := time.Now().Add(24 * time.Hour)
	if bl.Timestamp.After(cutoff) {
		return nil, fmt.Errorf("%s was too old", cutoff)
	}

	return bl.Results, nil
}

func (p *DiskPersister) Set(ctx context.Context, key string, rs []Result) error {
	kp := p.keyPath(key)
	klog.Infof("setting %s ...", kp)
	bl := &Blob{
		Timestamp: time.Now(),
		Results:   rs,
	}

	var bs bytes.Buffer
	enc := gob.NewEncoder(&bs)
	err := enc.Encode(bl)
	if err != nil {
		return fmt.Errorf("encode: %v", err)
	}

	return os.WriteFile(kp, bs.Bytes(), 0o700)
}

type FirePersister struct {
	client *firestore.Client
}

func NewFirePersister(ctx context.Context) (*FirePersister, error) {
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("client: %v", err)
	}

	return &FirePersister{client: client}, nil
}

func (p *FirePersister) keyPath(key string) string {
	kr := regexp.MustCompile(`\W+`)
	key = kr.ReplaceAllString(key, "_")
	return fmt.Sprintf("repos/%s", key)
}

func (p *FirePersister) Get(ctx context.Context, key string) ([]Result, error) {
	kp := p.keyPath(key)
	klog.Infof("checking %s ...", kp)

	doc := p.client.Doc(kp)
	docsnap, err := doc.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}

	data, err := docsnap.DataAt("blob")
	if err != nil {
		return nil, fmt.Errorf("data at: %w", err)
	}

	bs, ok := data.([]byte)
	if !ok {
		return nil, fmt.Errorf("blob is not []byte")
	}

	bl := &Blob{}
	enc := gob.NewDecoder(bytes.NewReader(bs))
	err = enc.Decode(&bl)
	if err != nil {
		return nil, fmt.Errorf("decode fail: %v", err)
	}

	cutoff := time.Now().Add(24 * time.Hour)
	if bl.Timestamp.After(cutoff) {
		return nil, fmt.Errorf("%s was too old", cutoff)
	}

	return bl.Results, nil
}

func (p *FirePersister) Set(ctx context.Context, key string, rs []Result) error {
	kp := p.keyPath(key)
	klog.Infof("setting %s ...", kp)
	bl := &Blob{
		Timestamp: time.Now(),
		Results:   rs,
	}

	var bs bytes.Buffer
	enc := gob.NewEncoder(&bs)
	err := enc.Encode(bl)
	if err != nil {
		return fmt.Errorf("encode: %v", err)
	}

	if _, err := p.client.Doc(kp).Set(ctx, map[string]interface{}{"blob": bs.Bytes()}); err != nil {
		return fmt.Errorf("set: %w", err)
	}
	return nil
}
