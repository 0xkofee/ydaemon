package vaults

import (
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/yearn/ydaemon/internal/meta"
	"github.com/yearn/ydaemon/internal/prices"
	"github.com/yearn/ydaemon/internal/strategies"
	"github.com/yearn/ydaemon/internal/utils/helpers"
	"github.com/yearn/ydaemon/internal/utils/models"
)

func buildVaultName(
	chainID uint64,
	vaultAddress common.Address,
	vaultName string,
	metaVaultName string,
	tokenName string,
) (name string, displayName string, formatedName string) {
	name = strings.Replace(vaultName, "\"", "", -1)
	formatedName = tokenName
	if metaVaultName != "" {
		displayName = metaVaultName
	} else {
		vaultFromMeta, ok := meta.Store.VaultsFromMeta[chainID][vaultAddress]
		if ok {
			displayName = strings.Replace(vaultFromMeta.DisplayName, "\"", "", -1)
		}
	}

	//If the formated name is missing yVault suffix, add it
	if !strings.HasSuffix(formatedName, "yVault") {
		formatedName = formatedName + " yVault"
	}
	// If a display name exist, use it for the formating.
	if displayName != "" && !strings.HasSuffix(displayName, "yVault") {
		formatedName = displayName + " yVault"
	}
	// If the name is empty, use the displayName instead
	if name == "" {
		name = displayName
	}
	// If the name is still empty, use the formated name instead
	if name == "" {
		name = formatedName
	}

	return name, displayName, formatedName
}

func buildVaultSymbol(
	chainID uint64,
	tokenAddress common.Address,
	vaultSymbol string,
	tokenSymbol string,
) (symbol string, displaySymbol string, formatedSymbol string) {
	symbol = strings.Replace(vaultSymbol, "\"", "", -1)
	formatedSymbol = tokenSymbol
	shareTokenFromMeta, ok := meta.Store.TokensFromMeta[chainID][tokenAddress]
	if ok {
		displaySymbol = strings.Replace(shareTokenFromMeta.Symbol, "\"", "", -1)
	}

	//If the formated symbol is missing yv prefix, add it
	if !strings.HasPrefix(formatedSymbol, "yv") {
		formatedSymbol = "yv" + formatedSymbol
	}
	// If a display name exist, use it for the formating.
	if displaySymbol != "" && !strings.HasPrefix(displaySymbol, "yv") {
		formatedSymbol = "yv" + displaySymbol
	}
	symbol = helpers.ValueWithFallback(symbol, displaySymbol)
	symbol = helpers.ValueWithFallback(symbol, formatedSymbol)
	displaySymbol = helpers.ValueWithFallback(displaySymbol, symbol)

	return symbol, displaySymbol, formatedSymbol
}

// Get the price of the underlying asset. This is tricky because of the decimals. The prices are fetched
// using the lens oracle daemon, stored in the TokenPrices map, and based on the USDC token, aka with 6
// decimals. We first need to parse the BigInt Price to a float64, then divide by 10^6 to get the price
// in an human readable USDC format.
func buildTokenPrice(chainID uint64, tokenAddress common.Address) (*big.Float, float64) {
	prices := prices.Store.TokenPrices[chainID]
	fPrice := new(big.Float)
	price, ok := prices[tokenAddress]
	if ok {
		fPrice.SetInt(price)
		humanizedPrice := new(big.Float).Quo(fPrice, big.NewFloat(math.Pow10(int(6))))
		fHumanizedPrice, _ := humanizedPrice.Float64()
		return humanizedPrice, fHumanizedPrice
	}
	return big.NewFloat(0), 0.0
}

// Get the total assets locked in this vault. This is tricky because of the decimals. The total asset value
// is a string which will be formated as a float64 and scaled with the underlying token decimals. With that
// we should have a human readable Total Asset value, and we should be able to get the Total Value Locked
// in the vault thanks to the above humanizedPrice value.
func buildTVL(balanceToken string, decimals int, humanizedPrice *big.Float) float64 {
	_, humanizedTVL := helpers.FormatAmount(balanceToken, decimals)
	fHumanizedTVLPrice, _ := big.NewFloat(0).Mul(humanizedTVL, humanizedPrice).Float64()
	return fHumanizedTVLPrice
}

// From the legacy API, build the schema for the APY, models.TAPY, used to get the details and the
// breakdown of the vault.
func buildAPY(
	chainID uint64,
	vaultAddress common.Address,
	perfFee,
	manaFee uint64,
	override string,
) TAPY {
	apy := TAPY{}
	apyFromAPIV1, ok := Store.VaultsFromAPIV1[chainID][vaultAddress]

	if ok {
		apy = TAPY{
			Type:     apyFromAPIV1.APY.Type,
			GrossAPR: apyFromAPIV1.APY.GrossAPR,
			NetAPY:   apyFromAPIV1.APY.NetAPY,
			Points: TAPYPoints{
				WeekAgo:   apyFromAPIV1.APY.Points.WeekAgo,
				MonthAgo:  apyFromAPIV1.APY.Points.MonthAgo,
				Inception: apyFromAPIV1.APY.Points.Inception,
			},
			Composite: TAPYComposite{
				Boost:      apyFromAPIV1.APY.Composite.Boost,
				PoolAPY:    apyFromAPIV1.APY.Composite.PoolAPY,
				BoostedAPR: apyFromAPIV1.APY.Composite.BoostedAPR,
				BaseAPR:    apyFromAPIV1.APY.Composite.BaseAPR,
				CvxAPR:     apyFromAPIV1.APY.Composite.CvxAPR,
				RewardsAPR: apyFromAPIV1.APY.Composite.RewardsAPR,
			},
			Fees: TAPYFees{
				Performance: float64(perfFee) / 10000,
				Management:  float64(manaFee) / 10000,
				Withdrawal:  apyFromAPIV1.APY.Fees.Withdrawal,
				KeepCRV:     apyFromAPIV1.APY.Fees.KeepCRV,
				CvxKeepCRV:  apyFromAPIV1.APY.Fees.CvxKeepCRV,
			},
		}
	}
	if override != "" {
		apy.Type = override
	}
	return apy
}

// Get the migration informations for this vault. By default, migrationAvailable is false and
// the migrationAddress matches the vault address. If a migration is available, point this last
// one to MigrationTargetVault.
func buildMigration(chainID uint64, vaultAddress common.Address) TMigration {
	migration := TMigration{}
	vaultFromMeta, ok := meta.Store.VaultsFromMeta[chainID][vaultAddress]

	if ok {
		migrationAddress := vaultAddress.String()
		migrationAvailable := vaultFromMeta.MigrationAvailable
		if vaultFromMeta.MigrationAvailable {
			migrationAddress = common.HexToAddress(vaultFromMeta.MigrationTargetVault).String()
		}

		migration = TMigration{
			Available: migrationAvailable,
			Address:   migrationAddress,
		}
	}
	return migration
}

func prepareVaultSchema(
	chainID uint64,
	strategiesCondition string,
	withStrategiesRisk bool,
	withStrategiesDetails bool,
	vaultFromGraph models.TVaultFromGraph,
) *TVault {
	chainIDAsString := strconv.FormatUint(chainID, 10)
	vaultAddress := common.HexToAddress(vaultFromGraph.Id)
	tokenAddress := common.HexToAddress(vaultFromGraph.Token.Id)
	tokenFromMeta := meta.Store.TokensFromMeta[chainID][tokenAddress]
	updated := helpers.StrToUint(vaultFromGraph.LatestUpdate.Timestamp, 0)
	activation := helpers.StrToUint(vaultFromGraph.Activation, 0)
	tokenDisplayName := helpers.ValueWithFallback(tokenFromMeta.Name, vaultFromGraph.Token.Name)
	tokenDisplaySymbol := helpers.ValueWithFallback(tokenFromMeta.Symbol, vaultFromGraph.Token.Symbol)
	vaultFromMeta, ok := meta.Store.VaultsFromMeta[chainID][vaultAddress]
	if !ok {
		// If the vault file is missing, we set the default values for its fields
		vaultFromMeta = meta.TVaultFromMeta{
			Order:               1000000000,
			HideAlways:          false,
			DepositsDisabled:    false,
			WithdrawalsDisabled: false,
			MigrationAvailable:  false,
			AllowZapIn:          true,
			AllowZapOut:         true,
			Retired:             false,
		}
	}

	vaultName, vaultDisplayName, vaultFormatedName := buildVaultName(
		chainID,
		vaultAddress,
		vaultFromGraph.ShareToken.Name,
		vaultFromMeta.DisplayName,
		vaultFromGraph.Token.Name,
	)
	vaultSymbol, vaultDisplaySymbol, vaultFormatedSymbol := buildVaultSymbol(
		chainID,
		common.HexToAddress(vaultFromGraph.ShareToken.Id),
		vaultFromGraph.ShareToken.Symbol,
		vaultFromGraph.Token.Symbol,
	)
	humanizedPrice, fHumanizedPrice := buildTokenPrice(
		chainID,
		tokenAddress,
	)

	strategies := strategies.BuildStrategies(
		chainID,
		withStrategiesDetails,
		withStrategiesRisk,
		strategiesCondition,
		humanizedPrice,
		vaultFromGraph,
	)

	fHumanizedTVLPrice := buildTVL(
		vaultFromGraph.BalanceTokens,
		int(vaultFromGraph.Token.Decimals),
		humanizedPrice,
	)
	delegatedTokenAsBN := big.NewInt(0)
	fDelegatedValue := 0.0

	for _, strat := range strategies {
		stratDelegatedValueAsFloat, err := strconv.ParseFloat(strat.DelegatedValue, 64)
		if err == nil {
			stratDelegatedTokenAsBN, ok := big.NewInt(0).SetString(strat.DelegatedAssets, 10)
			if ok {
				delegatedTokenAsBN = delegatedTokenAsBN.Add(delegatedTokenAsBN, stratDelegatedTokenAsBN)
				fDelegatedValue += stratDelegatedValueAsFloat
			}
		}
	}

	vault := &TVault{
		Inception:      activation,
		Address:        vaultAddress.String(),
		Symbol:         vaultSymbol,
		DisplaySymbol:  vaultDisplaySymbol,
		FormatedSymbol: vaultFormatedSymbol,
		Name:           vaultName,
		DisplayName:    vaultDisplayName,
		FormatedName:   vaultFormatedName,
		Icon:           helpers.GITHUB_ICON_BASE_URL + chainIDAsString + `/` + vaultAddress.String() + `/logo-128.png`,
		Token: TToken{
			Address:     common.HexToAddress(vaultFromGraph.Token.Id).String(),
			Name:        vaultFromGraph.Token.Name,
			DisplayName: tokenDisplayName,
			Symbol:      tokenDisplaySymbol,
			Description: tokenFromMeta.Description,
			Decimals:    vaultFromGraph.Token.Decimals,
			Icon:        helpers.GITHUB_ICON_BASE_URL + chainIDAsString + `/` + common.HexToAddress(vaultFromGraph.Token.Id).String() + `/logo-128.png`,
		},
		TVL: TTVL{
			TotalAssets:          vaultFromGraph.BalanceTokens,
			TotalDelegatedAssets: delegatedTokenAsBN.String(),
			TVL:                  fHumanizedTVLPrice - fDelegatedValue,
			TVLDeposited:         fHumanizedTVLPrice,
			TVLDelegated:         fDelegatedValue,
			Price:                fHumanizedPrice,
		},
		Details: &TVaultDetails{
			Management:            vaultFromGraph.Management,
			Governance:            vaultFromGraph.Governance,
			Guardian:              vaultFromGraph.Guardian,
			Rewards:               vaultFromGraph.Rewards,
			DepositLimit:          vaultFromGraph.DepositLimit,
			Comment:               vaultFromMeta.Comment,
			AvailableDepositLimit: vaultFromGraph.AvailableDepositLimit,
			Order:                 vaultFromMeta.Order,
			PerformanceFee:        vaultFromGraph.PerformanceFeeBps,
			ManagementFee:         vaultFromGraph.ManagementFeeBps,
			DepositsDisabled:      vaultFromMeta.DepositsDisabled,
			WithdrawalsDisabled:   vaultFromMeta.WithdrawalsDisabled,
			AllowZapIn:            vaultFromMeta.AllowZapIn,
			AllowZapOut:           vaultFromMeta.AllowZapOut,
			Retired:               vaultFromMeta.Retired,
		},
		APY: buildAPY(
			chainID,
			vaultAddress,
			vaultFromGraph.PerformanceFeeBps,
			vaultFromGraph.ManagementFeeBps,
			vaultFromMeta.APYTypeOverride,
		),
		Strategies: strategies,
		Endorsed:   vaultFromGraph.Classification == "Endorsed",
		Version:    vaultFromGraph.ApiVersion,
		Decimals:   vaultFromGraph.ShareToken.Decimals,
		Type:       "v2", //No v1 in the subgraph
		// Emergency_shutdown: ,
		Updated:   updated / 1000,
		Migration: buildMigration(chainID, vaultAddress),
	}

	return vault
}
