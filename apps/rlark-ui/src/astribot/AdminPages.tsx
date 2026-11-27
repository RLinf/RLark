import { useState } from "react";
import type { Copy } from "../i18n";
import type { IssuedNodeAdmission } from "./lib/siteNodeDisplay";
import type { NodeJoinTarget } from "./lib/nodeJoin";
import { JoinNodePage } from "./pages/JoinNode";
import { StationsPage } from "./pages/Stations";
import { SiteNetworkSettingsPage } from "./pages/SiteNetworkSettings";
import { RobotModelsPage } from "./pages/RobotModels";
import { IAMPage } from "./iam/IAMPage";

// The admin pages this fork adds. AdminApp always renders it, so its state lives as long as AdminApp's.
export function AstribotAdminPages({ page, sub, copy, zh, navigate }: {
  page: string;
  sub: string;
  copy: Copy;
  zh: boolean;
  navigate: (id: string, sub?: string) => void;
}) {
  // A reissued code, shown once on the page that adds a node.
  const [reissued, setReissued] = useState<IssuedNodeAdmission>();
  // An SSH join to show on that page, or the target a station's Rejoin opens (from the admission records, or the
  // station's own cluster, node name and mapped login).
  const [sshJoin, setSSHJoin] = useState<NodeJoinTarget>();
  if (page === "iam") return <IAMPage zh={zh} />;
  if (page !== "stations") return null;
  if (sub === "network") return <SiteNetworkSettingsPage zh={zh} navigate={navigate} />;
  if (sub === "models") return <RobotModelsPage zh={zh} navigate={navigate} />;
  if (sub !== "join") {
    return (
      <StationsPage
        copy={copy}
        onModels={() => navigate("stations", "models")}
        onJoin={(issued, join) => {
          setReissued(issued);
          setSSHJoin(join);
          navigate("stations", "join");
        }}
      />
    );
  }
  return (
    <JoinNodePage
      zh={zh}
      reissued={reissued}
      sshJoin={sshJoin}
      onScanSettings={() => navigate("stations", "network")}
      onBack={() => {
        setReissued(undefined);
        setSSHJoin(undefined);
        navigate("stations");
      }}
    />
  );
}
