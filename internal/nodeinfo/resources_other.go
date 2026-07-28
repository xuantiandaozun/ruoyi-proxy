//go:build !linux

package nodeinfo

func collectPlatformResources() Resources {
	return Resources{}
}
