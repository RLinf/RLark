import { useState } from "react";

// What a form sends: run marks it pending, clears the last failure, waits for work and keeps its failure's message.
export function useAction() {
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const run = async (work: () => Promise<unknown>) => {
    setPending(true);
    setError("");
    try {
      await work();
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setPending(false);
    }
  };
  return { pending, error, setError, run };
}
