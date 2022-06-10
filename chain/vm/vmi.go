package vm

import (
	"context"
	"os"
	"sync"

	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/ipfs/go-cid"
)

type Interface interface {
	// Applies the given message onto the VM's current state, returning the result of the execution
	ApplyMessage(ctx context.Context, cmsg types.ChainMsg) (*ApplyRet, error)
	// Same as above but for system messages (the Cron invocation and block reward payments).
	// Must NEVER fail.
	ApplyImplicitMessage(ctx context.Context, msg *types.Message) (*ApplyRet, error)
	// Flush all buffered objects into the state store provided to the VM at construction.
	Flush(ctx context.Context) (cid.Cid, error)
}

var experimentalUseFvm = os.Getenv("LOTUS_USE_FVM_TO_SYNC_MAINNET_V15") == "1"

func NewVM(ctx context.Context, opts *VMOpts) (Interface, error) {
	if opts.NetworkVersion >= network.Version16 {
		if os.Getenv("LOTUS_FVM_DEBUG") == "1" {
			return NewDualExecutionVM(ctx, opts)
		}
		return NewFVM(ctx, opts)
	}

	// Remove after v16 upgrade, this is only to support testing and validation of the FVM
	if experimentalUseFvm && opts.NetworkVersion >= network.Version15 {
		if os.Getenv("LOTUS_FVM_DEBUG") == "1" {
			return NewDualExecutionVM(ctx, opts)
		}
		return NewFVM(ctx, opts)
	}

	return NewLegacyVM(ctx, opts)
}

type dualExecutionVM struct {
	main  *FVM
	debug *FVM
}

var _ Interface = (*dualExecutionVM)(nil)

func NewDualExecutionVM(ctx context.Context, opts *VMOpts) (Interface, error) {
	main, err := NewFVM(ctx, opts)
	if err != nil {
		return nil, err
	}

	debug, err := NewDebugFVM(ctx, opts)
	if err != nil {
		return nil, err
	}

	return &dualExecutionVM{
		main:  main,
		debug: debug,
	}, nil
}

func (vm *dualExecutionVM) ApplyMessage(ctx context.Context, cmsg types.ChainMsg) (ret *ApplyRet, err error) {
	var wg sync.WaitGroup

	wg.Add(2)

	go func() {
		defer wg.Done()
		ret, err = vm.main.ApplyMessage(ctx, cmsg)
	}()

	go func() {
		defer wg.Done()
		if _, err := vm.debug.ApplyMessage(ctx, cmsg); err != nil {
			log.Errorf("debug execution failed: %w", err)
		}
	}()

	wg.Wait()
	return ret, err
}

func (vm *dualExecutionVM) ApplyImplicitMessage(ctx context.Context, msg *types.Message) (ret *ApplyRet, err error) {
	var wg sync.WaitGroup

	wg.Add(2)

	go func() {
		defer wg.Done()
		ret, err = vm.main.ApplyImplicitMessage(ctx, msg)
	}()

	go func() {
		defer wg.Done()
		if _, err := vm.debug.ApplyImplicitMessage(ctx, msg); err != nil {
			log.Errorf("debug execution failed: %w", err)
		}
	}()

	wg.Wait()
	return ret, err
}

func (vm *dualExecutionVM) Flush(ctx context.Context) (cid.Cid, error) {
	return vm.main.Flush(ctx)
}
