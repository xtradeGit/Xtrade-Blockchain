package keeper

import (
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/params"

	"github.com/tharsis/ethermint/x/evm/types"
)

// RefundGasFn defines a custom gas refund function
type RefundGasFn func(ctx sdk.Context, msg core.Message, leftoverGas uint64, denom string) error

// HasRefundGasFn a
func (k Keeper) HasRefundGasFn() bool {
	return k.refundGas != nil
}

// SetRefundGasFn sets the refund logic to the
func (k *Keeper) SetRefundGasFn(fn RefundGasFn) {
	if k.HasRefundGasFn() {
		panic("gas refund handler already set")
	}
	k.refundGas = fn
}

// RefundGas defines the default RefundGasFn logic. It transfers the leftover gas to the sender of the
// message, caped to half of the total gas consumed in the transaction. Additionally, the function sets
// the total gas consumed to the value returned by the EVM execution, thus ignoring the previous
// intrinsic gas consumed during in the AnteHandler.
func (k *Keeper) RefundGas() RefundGasFn {
	return func(ctx sdk.Context, msg core.Message, leftoverGas uint64, denom string) error {
		// Return EVM tokens for remaining gas, exchanged at the original rate.
		remaining := new(big.Int).Mul(new(big.Int).SetUint64(leftoverGas), msg.GasPrice())

		switch remaining.Sign() {
		case -1:
			// negative refund errors
			return sdkerrors.Wrapf(types.ErrInvalidRefund, "refunded amount value cannot be negative %d", remaining.Int64())
		case 1:
			// positive amount refund
			refundedCoins := sdk.Coins{sdk.NewCoin(denom, sdk.NewIntFromBigInt(remaining))}

			// refund to sender from the fee collector module account, which is the escrow account in charge of collecting tx fees

			err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, authtypes.FeeCollectorName, msg.From().Bytes(), refundedCoins)
			if err != nil {
				err = sdkerrors.Wrapf(sdkerrors.ErrInsufficientFunds, "fee collector account failed to refund fees: %s", err.Error())
				return sdkerrors.Wrapf(err, "failed to refund %d leftover gas (%s)", leftoverGas, refundedCoins.String())
			}
		default:
			// no refund, consume gas and update the tx gas meter
		}

		return nil
	}
}

// GetEthIntrinsicGas returns the intrinsic gas cost for the transaction
func (k *Keeper) GetEthIntrinsicGas(ctx sdk.Context, msg core.Message, cfg *params.ChainConfig, isContractCreation bool) (uint64, error) {
	height := big.NewInt(ctx.BlockHeight())
	homestead := cfg.IsHomestead(height)
	istanbul := cfg.IsIstanbul(height)

	return core.IntrinsicGas(msg.Data(), msg.AccessList(), isContractCreation, homestead, istanbul)
}

// ResetGasMeterAndConsumeGas reset first the gas meter consumed value to zero and set it back to the new value
// 'gasUsed'
func (k *Keeper) ResetGasMeterAndConsumeGas(ctx sdk.Context, gasUsed uint64) {
	// reset the gas count
	ctx.GasMeter().RefundGas(ctx.GasMeter().GasConsumed(), "reset the gas count")
	ctx.GasMeter().ConsumeGas(gasUsed, "apply evm transaction")
}
