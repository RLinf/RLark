import { Check, CircleAlert, LoaderCircle, Minus } from "lucide-react";
import type { StageView } from "../lib/clusterProvision";
import "./provision.css";

// StageList is the stages of a making or a join as they stand: each one's state, what it read back, and the
// message of the one running (its latest line) or the one that stopped (why).
export function StageList({ stages, zh }: { stages: StageView[]; zh: boolean }) {
  return (
    <ol className="provision-stages">
      {stages.map((stage) => (
        <li key={stage.name} className={`provision-stage provision-stage-${stage.state.toLowerCase()}`}>
          <span className="provision-stage-icon">
            {stage.state === "Done" && <Check size={15} />}
            {stage.state === "Running" && <LoaderCircle size={15} className="job-action-loading" />}
            {stage.state === "Skipped" && <Minus size={15} />}
            {stage.state === "Failed" && <CircleAlert size={15} />}
          </span>
          <div>
            <strong>
              {stage.label}
              {stage.state === "Skipped" && <span className="provision-stage-skip">{zh ? "跳过" : "skipped"}</span>}
            </strong>
            {stage.readback && <small>{stage.readback}</small>}
            {stage.message && <small className="provision-stage-message">{stage.message}</small>}
          </div>
        </li>
      ))}
    </ol>
  );
}
