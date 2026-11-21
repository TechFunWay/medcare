export function isFnOSGatewayOrigin(): boolean {
  return (window as any).__FNOS_GATEWAY__ === true
}
