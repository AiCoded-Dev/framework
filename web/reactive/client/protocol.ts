export interface Binding {
  kind: "text" | "html";
  value: string;
}

export interface InitFrame {
  t: "init";
  bindings: Record<string, Binding>;
}

export interface PatchFrame extends Binding {
  t: "patch";
  key: string;
}

export interface AckFrame {
  t: "ack";
  routeKey: string;
  var: string;
}

export interface ErrFrame {
  t: "err";
  routeKey: string;
  var: string;
  msg: string;
  code: string;
}

export interface ResultFrame {
  t: "result";
  id: number;
  value: unknown;
}

export interface FailFrame {
  t: "fail";
  id: number;
  status: number;
  msg: string;
}

export type ServerFrame = InitFrame | PatchFrame | AckFrame | ErrFrame | ResultFrame | FailFrame;

export interface WriteFrame {
  t: "write";
  routeKey: string;
  var: string;
  value: unknown;
}

export interface CallFrame {
  t: "call";
  id: number;
  routeKey: string;
  name: string;
  args: unknown;
}

export interface CallError {
  status: number;
  message: string;
}

// StatusReauth closes a connection that reached its maximum age; reconnect at once.
export const StatusReauth = 4000;
