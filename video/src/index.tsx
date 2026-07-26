import React from 'react';
import {Composition, registerRoot} from 'remotion';
import {CliWorkflow} from './CliWorkflow';
import {SelfContainedInstall} from './SelfContainedInstall';

export const Root: React.FC = () => (
  <>
    <Composition
      id="CliWorkflow"
      component={CliWorkflow}
      durationInFrames={390}
      fps={30}
      width={1920}
      height={1080}
    />
    <Composition
      id="SelfContainedInstall"
      component={SelfContainedInstall}
      durationInFrames={390}
      fps={30}
      width={1920}
      height={1080}
    />
  </>
);

registerRoot(Root);
