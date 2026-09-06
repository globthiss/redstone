package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"redstone/hashutil"
	"redstone/modes"
)

const defaultMaxFileSize = 2 << 30

type Task struct {
	ID         string
	URLs       []string
	Dest       string
	SHA1       string
	Size       int64
	MaxSize    int64
	Executable bool
}

type Progress struct {
	TaskID       string
	BytesDone    int64
	BytesTotal   int64
	FilesDone    int32
	FilesTotal   int32
	CurrentFile  string
	Err          error
	SkippedCache bool
}

type Options struct {
	Workers    int
	Retries    int
	Timeout    time.Duration
	HTTPClient *http.Client
	Mode       modes.DownloadMode
	ProxyURL   string
}

func parseProxy(raw string) (*url.URL, error) {
	return url.Parse(raw)
}

func (o *Options) fill() {
	if o.Workers <= 0 {
		o.Workers = 6
	}
	if o.Mode == modes.DownloadModeSequential {
		o.Workers = 1
	}
	if o.Retries <= 0 {
		o.Retries = 3
	}
	if o.Timeout <= 0 {
		o.Timeout = 30 * time.Second
	}
	if o.HTTPClient == nil {
		transport := &http.Transport{}
		if o.ProxyURL != "" {
			if proxyURL, err := parseProxy(o.ProxyURL); err == nil {
				transport.Proxy = http.ProxyURL(proxyURL)
			}
		}
		o.HTTPClient = &http.Client{Timeout: o.Timeout, Transport: transport}
	}
}

func Pool(ctx context.Context, tasks []Task, opts Options, progress chan<- Progress) []error {
	opts.fill()
	if len(tasks) == 0 {
		return nil
	}

	if opts.Mode == modes.DownloadModeMirrorPriority {
		tasks = reorderMirrorFirst(tasks)
	}

	var total int64
	for _, t := range tasks {
		total += t.Size
	}

	var (
		filesDone int32
		bytesDone int64
		errsMu    sync.Mutex
		errs      []error
		wg        sync.WaitGroup
		sem       = make(chan struct{}, opts.Workers)
	)

	emit := func(p Progress) {
		if progress == nil {
			return
		}
		select {
		case progress <- p:
		case <-ctx.Done():
		}
	}

	for _, task := range tasks {
		task := task

		if ctx.Err() != nil {
			errsMu.Lock()
			errs = append(errs, fmt.Errorf("%s: %w", task.ID, ctx.Err()))
			errsMu.Unlock()
			continue
		}

		wg.Add(1)
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Done()
			errsMu.Lock()
			errs = append(errs, fmt.Errorf("%s: %w", task.ID, ctx.Err()))
			errsMu.Unlock()
			continue
		}

		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			if ctx.Err() != nil {
				return
			}

			if hashutil.VerifyFile(task.Dest, task.SHA1, task.Size) {
				atomic.AddInt32(&filesDone, 1)
				atomic.AddInt64(&bytesDone, task.Size)
				emit(Progress{
					TaskID: task.ID, CurrentFile: task.Dest,
					BytesDone: atomic.LoadInt64(&bytesDone), BytesTotal: total,
					FilesDone: atomic.LoadInt32(&filesDone), FilesTotal: int32(len(tasks)),
					SkippedCache: true,
				})
				return
			}

			if opts.Mode == modes.DownloadModeCacheOnly {
				err := fmt.Errorf("файл отсутствует в кэше, а режим cache_only запрещает загрузку из сети")
				errsMu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", task.ID, err))
				errsMu.Unlock()
				emit(Progress{TaskID: task.ID, CurrentFile: task.Dest, Err: err})
				return
			}

			err := downloadWithFallback(ctx, task, opts)
			if err != nil {
				errsMu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", task.ID, err))
				errsMu.Unlock()
				emit(Progress{TaskID: task.ID, CurrentFile: task.Dest, Err: err})
				return
			}

			atomic.AddInt32(&filesDone, 1)
			atomic.AddInt64(&bytesDone, task.Size)
			emit(Progress{
				TaskID: task.ID, CurrentFile: task.Dest,
				BytesDone: atomic.LoadInt64(&bytesDone), BytesTotal: total,
				FilesDone: atomic.LoadInt32(&filesDone), FilesTotal: int32(len(tasks)),
			})
		}()
	}

	wg.Wait()
	return errs
}

func reorderMirrorFirst(tasks []Task) []Task {
	out := make([]Task, len(tasks))
	for i, t := range tasks {
		if len(t.URLs) > 1 {
			reordered := append([]string{}, t.URLs[1:]...)
			reordered = append(reordered, t.URLs[0])
			t.URLs = reordered
		}
		out[i] = t
	}
	return out
}

func downloadWithFallback(ctx context.Context, task Task, opts Options) error {
	if len(task.URLs) == 0 {
		return errors.New("нет URL для загрузки")
	}

	maxSize := task.MaxSize
	if maxSize <= 0 {
		if task.Size > 0 {
			maxSize = task.Size + task.Size/10 + 1024
		} else {
			maxSize = defaultMaxFileSize
		}
	}

	var lastErr error
	for _, url := range task.URLs {
		for attempt := 1; attempt <= opts.Retries; attempt++ {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			err := downloadOnce(ctx, opts.HTTPClient, url, task.Dest, maxSize)
			if err == nil {
				if task.SHA1 != "" || task.Size > 0 {
					if !hashutil.VerifyFile(task.Dest, task.SHA1, task.Size) {
						lastErr = fmt.Errorf("проверка контрольной суммы не пройдена: %s", url)
						os.Remove(task.Dest)
						continue
					}
				}
				if task.Executable {
					os.Chmod(task.Dest, 0o755)
				}
				return nil
			}
			lastErr = err
			select {
			case <-time.After(time.Duration(attempt) * 300 * time.Millisecond):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return fmt.Errorf("не удалось скачать после перебора всех зеркал: %w", lastErr)
}

func downloadOnce(ctx context.Context, client *http.Client, targetURL, dest string, maxSize int64) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "RedstoneCore/1.0 (+launcher-core)")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d от %s", resp.StatusCode, targetURL)
	}

	if resp.ContentLength > 0 && resp.ContentLength > maxSize {
		return fmt.Errorf("файл превышает допустимый размер (%d > %d байт)", resp.ContentLength, maxSize)
	}

	tmp := dest + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}

	limited := io.LimitReader(resp.Body, maxSize+1)
	written, err := io.Copy(out, limited)
	if err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if written > maxSize {
		out.Close()
		os.Remove(tmp)
		return fmt.Errorf("файл превышает допустимый размер лимита %d байт", maxSize)
	}

	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}

	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func VerifyCached(t Task) bool {
	return hashutil.VerifyFile(t.Dest, t.SHA1, t.Size)
}

func Ping(ctx context.Context, targetURL string, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500
}
