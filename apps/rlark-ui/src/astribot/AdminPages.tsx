import { useState } from "react";
import type { Copy } from "../i18n";
import type { IssuedNodeAdmission } from "./lib/siteNodeDisplay";
import { JoinNodePage } from "./pages/JoinNode";
import { StationsPage } from "./pages/Stations";

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
  // An SSH join to show on that page, from the admission records.
  const [sshJoin, setSSHJoin] = useState<{ clusterID: string; name: string }>();
  if (page !== "stations") return null;
  if (sub !== "join") {
    return (
      <StationsPage
        copy={copy}
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
      onBack={() => {
        setReissued(undefined);
        setSSHJoin(undefined);
        navigate("stations");
      }}
    />
  );
}
