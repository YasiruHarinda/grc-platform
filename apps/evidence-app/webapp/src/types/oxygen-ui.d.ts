// Shim for @oxygen-ui/react-icons.
// Their package.json `exports` field doesn't expose the bundled .d.ts file,
// so TypeScript (with moduleResolution: "bundler") can't find types.
// This tells TS the module exists and treats its imports as untyped — safe
// since we only use it for visual <Icon /> components.
declare module "@oxygen-ui/react-icons";
